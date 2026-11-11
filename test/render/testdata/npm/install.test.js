"use strict";

const assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const { EventEmitter } = require("node:events");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { pathToFileURL } = require("node:url");
const zlib = require("node:zlib");

// The Go test renders the package for the fixture tool and stages a mirror laid out like a
// release: <mirror>/<archive>.
const pkgDir = process.env.SPINE_NPM_PACKAGE;
const mirrorDir = process.env.SPINE_NPM_MIRROR;
const installer = require(path.join(pkgDir, "install.js"));

function tarEntry(name, body = "", typeflag = "0") {
  const header = Buffer.alloc(512);
  header.write(name, 0, "utf8");
  header.write("0000755\0", 100);
  header.write(
    `${Buffer.byteLength(body).toString(8).padStart(11, "0")}\0`,
    124,
  );
  header.write("        ", 148);
  header.write(typeflag, 156);
  const sum = header.reduce((total, byte) => total + byte, 0);
  header.write(`${sum.toString(8).padStart(6, "0")}\0 `, 148);
  const data = Buffer.alloc(Math.ceil(Buffer.byteLength(body) / 512) * 512);
  data.write(body);
  return Buffer.concat([header, data]);
}

function withSizeField(entry, field) {
  const copy = Buffer.from(entry);
  copy.write(field, 124, "latin1");
  return copy;
}

function tarGz(...entries) {
  return zlib.gzipSync(Buffer.concat([...entries, Buffer.alloc(1024)]));
}

function zipFile(...entries) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const { name, content, method = 8, flags = 0 } of entries) {
    const data =
      method === 8 ? zlib.deflateRawSync(content) : Buffer.from(content);
    const nameBuf = Buffer.from(name);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(flags, 6);
    local.writeUInt16LE(method, 8);
    local.writeUInt16LE(nameBuf.length, 26);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(flags, 8);
    central.writeUInt16LE(method, 10);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(Buffer.byteLength(content), 24);
    central.writeUInt16LE(nameBuf.length, 28);
    central.writeUInt32LE(offset, 42);
    locals.push(local, nameBuf, data);
    centrals.push(central, nameBuf);
    offset += local.length + nameBuf.length + data.length;
  }
  const directory = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, directory, end]);
}

const targets = [
  { platform: "darwin", arch: "arm64", want: "demo_1.2.3_darwin_arm64.tar.gz" },
  { platform: "darwin", arch: "x64", want: "demo_1.2.3_darwin_amd64.tar.gz" },
  { platform: "linux", arch: "arm64", want: "demo_1.2.3_linux_arm64.tar.gz" },
  { platform: "linux", arch: "x64", want: "demo_1.2.3_linux_amd64.tar.gz" },
  { platform: "win32", arch: "arm64", want: "demo_1.2.3_windows_arm64.zip" },
  { platform: "win32", arch: "x64", want: "demo_1.2.3_windows_amd64.zip" },
];

test("archiveName maps every release target", () => {
  for (const { platform, arch, want } of targets) {
    assert.equal(
      installer.archiveName(platform, arch),
      want,
      `${platform}/${arch}`,
    );
  }
});

test("archiveName rejects unsupported platforms", () => {
  const testcases = [
    ["freebsd", "x64"],
    ["linux", "ia32"],
    ["aix", "ppc64"],
  ];
  for (const [platform, arch] of testcases) {
    assert.throws(
      () => installer.archiveName(platform, arch),
      new RegExp(`unsupported platform ${platform}/${arch}`),
    );
  }
});

test("binaryPath keeps the binary inside the package under node_modules layouts", () => {
  const testcases = [
    ["linux", path.join("pkg", "vendor", "demo")],
    ["win32", path.join("pkg", "vendor", "demo.exe")],
  ];
  for (const [platform, want] of testcases) {
    assert.equal(installer.binaryPath("pkg", platform), want);
  }
});

test("fixCommand quotes the install script path for the platform", () => {
  const script = (dir) => path.join(dir, "install.js");
  const testcases = [
    {
      platform: "linux",
      dir: "/opt/my pkg",
      want: `node '${script("/opt/my pkg")}'`,
    },
    {
      platform: "linux",
      dir: "/opt/it's",
      want: `node '/opt/it'\\''s${path.sep}install.js'`,
    },
    {
      platform: "win32",
      dir: "C:\\Users\\me\\pkg",
      want: `node "${script("C:\\Users\\me\\pkg")}"`,
    },
  ];
  for (const tc of testcases) {
    assert.equal(
      installer.fixCommand(tc.dir, tc.platform),
      tc.want,
      `${tc.platform} ${tc.dir}`,
    );
  }
});

test("verifyChecksum accepts the release archive and rejects anything else", () => {
  const name = "demo_1.2.3_linux_amd64.tar.gz";
  const archive = fs.readFileSync(path.join(mirrorDir, name));
  installer.verifyChecksum(name, archive);
  assert.throws(
    () =>
      installer.verifyChecksum(
        name,
        Buffer.concat([archive, Buffer.from("x")]),
      ),
    /checksum mismatch for demo_1\.2\.3_linux_amd64\.tar\.gz/,
  );
  assert.throws(
    () => installer.verifyChecksum("other.tar.gz", archive),
    /no checksum is known for other\.tar\.gz/,
  );
  assert.throws(
    () => installer.verifyChecksum("constructor", archive),
    /no checksum is known for constructor/,
  );
});

test("extractTarGz returns the regular file named demo at the archive root", () => {
  const body = "binary bytes";
  const testcases = [
    {
      name: "plain root entry",
      archive: tarGz(tarEntry("demo", body)),
      want: body,
    },
    {
      name: "dot slash prefix",
      archive: tarGz(tarEntry("./demo", body)),
      want: body,
    },
    {
      name: "after a PAX header and other files",
      archive: tarGz(
        tarEntry("PaxHeader/demo", "x", "x"),
        tarEntry("README", "read me"),
        tarEntry("demo", body),
      ),
      want: body,
    },
    {
      name: "body larger than one block",
      archive: tarGz(tarEntry("demo", "z".repeat(1300))),
      want: "z".repeat(1300),
    },
    {
      name: "nested file is ignored",
      archive: tarGz(tarEntry("dir/demo", "nested")),
      error: /demo not found at the archive root/,
    },
    {
      name: "symlink is not a regular file",
      archive: tarGz(tarEntry("demo", "", "2")),
      error: /demo not found at the archive root/,
    },
    {
      name: "empty archive",
      archive: tarGz(),
      error: /demo not found at the archive root/,
    },
    {
      name: "truncated entry",
      archive: zlib.gzipSync(
        tarEntry("demo", "z".repeat(1300)).subarray(0, 700),
      ),
      error: /archive is truncated/,
    },
    {
      name: "not gzip",
      archive: Buffer.from("plain text"),
      error: /incorrect header check/,
    },
    {
      name: "size with a non-octal digit",
      archive: tarGz(withSizeField(tarEntry("demo", "x"), "0000000009\0\0")),
      error: /tar: malformed header/,
    },
    {
      name: "size with trailing garbage",
      archive: tarGz(withSizeField(tarEntry("demo", "x"), "00000000001x")),
      error: /tar: malformed header/,
    },
    {
      name: "size padded with spaces",
      archive: tarGz(withSizeField(tarEntry("demo", "x"), "       1 \0\0\0")),
      want: "x",
    },
  ];
  for (const tc of testcases) {
    if (tc.error) {
      assert.throws(
        () => installer.extractTarGz(tc.archive, "demo"),
        tc.error,
        tc.name,
      );
    } else {
      assert.equal(
        installer.extractTarGz(tc.archive, "demo").toString(),
        tc.want,
        tc.name,
      );
    }
  }
});

test("extractZip returns demo.exe and rejects unsupported entries", () => {
  const body = "windows binary bytes ".repeat(50);
  const testcases = [
    {
      name: "deflated",
      archive: zipFile({ name: "demo.exe", content: body }),
      want: body,
    },
    {
      name: "stored",
      archive: zipFile({ name: "demo.exe", content: body, method: 0 }),
      want: body,
    },
    {
      name: "after another entry",
      archive: zipFile(
        { name: "README", content: "read me" },
        { name: "demo.exe", content: body },
      ),
      want: body,
    },
    {
      name: "data descriptor flag",
      archive: zipFile({ name: "demo.exe", content: body, flags: 8 }),
      want: body,
    },
    {
      name: "unsupported method",
      archive: zipFile({ name: "demo.exe", content: body, method: 14 }),
      error: /compression method 14, which is not supported/,
    },
    {
      name: "encrypted",
      archive: zipFile({ name: "demo.exe", content: body, flags: 1 }),
      error: /demo\.exe is encrypted/,
    },
    {
      name: "missing entry",
      archive: zipFile({ name: "other.exe", content: body }),
      error: /demo\.exe not found/,
    },
    {
      name: "not a zip",
      archive: Buffer.from("plain text that is long enough to scan"),
      error: /end of central directory not found/,
    },
    {
      name: "truncated",
      archive: zipFile({ name: "demo.exe", content: body }).subarray(0, 60),
      error: /end of central directory not found/,
    },
  ];
  for (const tc of testcases) {
    if (tc.error) {
      assert.throws(
        () => installer.extractZip(tc.archive, "demo.exe"),
        tc.error,
        tc.name,
      );
    } else {
      assert.equal(
        installer.extractZip(tc.archive, "demo.exe").toString(),
        tc.want,
        tc.name,
      );
    }
  }
});

test("releaseUrl defaults to the GitHub release and only accepts https or file mirrors", () => {
  assert.equal(
    installer.releaseUrl({}),
    "https://github.com/scoutbin/demo/releases/download/v1.2.3",
  );
  assert.equal(
    installer.releaseUrl({
      DEMO_DOWNLOAD_BASE_URL: "https://mirror.example/dl//",
    }),
    "https://mirror.example/dl",
  );
  assert.equal(
    installer.releaseUrl({ DEMO_DOWNLOAD_BASE_URL: "file:///tmp/mirror" }),
    "file:///tmp/mirror",
  );
  for (const base of [
    "http://mirror.example",
    "ftp://mirror.example",
    "/tmp/mirror",
  ]) {
    assert.throws(
      () => installer.releaseUrl({ DEMO_DOWNLOAD_BASE_URL: base }),
      /must start with https:\/\/ or file:\/\//,
    );
  }
});

// Stands in for https.get: respond(href, callNumber) returns { status, location?, body? } or 'idle'.
function fakeGet(respond) {
  const urls = [];
  const get = (target, _options, onResponse) => {
    urls.push(target.href);
    const reply = respond(target.href, urls.length);
    const request = new EventEmitter();
    request.destroy = (err) => request.emit("error", err);
    setImmediate(() => {
      if (reply === "idle") return request.emit("timeout");
      const res = new EventEmitter();
      res.statusCode = reply.status;
      res.headers = reply.location ? { location: reply.location } : {};
      res.resume = () => {};
      onResponse(res);
      if (reply.status === 200) {
        res.emit("data", Buffer.isBuffer(reply.body) ? reply.body : Buffer.from(reply.body));
        res.emit("end");
      }
    });
    return request;
  };
  return { get, urls };
}

test("fetchBuffer follows https redirects and rejects unsafe or failing responses", async () => {
  const start = "https://example.test/start";
  const testcases = [
    {
      name: "redirect to http",
      respond: () => ({ status: 302, location: "http://example.test/plain" }),
      error: /refusing a redirect to http:/,
      calls: 1,
    },
    {
      name: "redirect loop",
      respond: () => ({ status: 302, location: start }),
      error: /too many redirects/,
      calls: installer.MAX_REDIRECTS + 1,
    },
    {
      name: "404",
      respond: () => ({ status: 404 }),
      error: /HTTP 404 for https:\/\/example\.test\/start/,
      calls: 1,
    },
    {
      name: "idle timeout",
      respond: () => "idle",
      error: /timed out fetching https:\/\/example\.test\/start/,
      calls: 1,
    },
    {
      name: "body over the size cap",
      respond: () => ({
        status: 200,
        body: Buffer.alloc(installer.MAX_DOWNLOAD_BYTES + 1),
      }),
      error: /is larger than 209715200 bytes/,
      calls: 1,
    },
    {
      name: "302 to https then 200",
      respond: (_href, n) =>
        n === 1
          ? { status: 302, location: "/final" }
          : { status: 200, body: "payload" },
      want: "payload",
      calls: 2,
    },
  ];
  for (const tc of testcases) {
    const { get, urls } = fakeGet(tc.respond);
    const result = installer.fetchBuffer(start, undefined, get);
    if (tc.error) {
      await assert.rejects(result, tc.error, tc.name);
    } else {
      assert.equal((await result).toString(), tc.want, tc.name);
    }
    assert.equal(urls.length, tc.calls, `${tc.name}: requests made`);
  }
});

function tempDir(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "demo-npm-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

const mirrorEnv = (mirror) => ({
  DEMO_DOWNLOAD_BASE_URL: pathToFileURL(mirror).href,
});

test("install verifies, extracts and writes the binary", async (t) => {
  const testcases = [
    {
      platform: "linux",
      arch: "x64",
      binaryName: "demo",
      content: "linux amd64 binary",
    },
    {
      platform: "darwin",
      arch: "arm64",
      binaryName: "demo",
      content: "darwin arm64 binary",
    },
    {
      platform: "win32",
      arch: "x64",
      binaryName: "demo.exe",
      content: "windows amd64 binary",
    },
  ];
  for (const tc of testcases) {
    const dir = path.join(tempDir(t), "pkg");
    await installer.install(dir, mirrorEnv(mirrorDir), tc.platform, tc.arch);
    const binary = installer.binaryPath(dir, tc.platform);
    assert.equal(path.basename(binary), tc.binaryName);
    assert.equal(fs.readFileSync(binary, "utf8"), tc.content);
    if (tc.platform !== "win32" && process.platform !== "win32") {
      assert.equal(fs.statSync(binary).mode & 0o777, 0o755);
    }
    assert.deepEqual(
      fs.readdirSync(path.dirname(binary)),
      [tc.binaryName],
      "no temp files left behind",
    );
  }
});

test("install leaves no binary when the release asset is tampered with, replaced or missing", async (t) => {
  const name = "demo_1.2.3_linux_amd64.tar.gz";
  const testcases = [
    {
      name: "tampered archive",
      change: (file) => fs.appendFileSync(file, "x"),
      error: /checksum mismatch/,
    },
    {
      name: "valid archive of other content",
      change: (file) => fs.writeFileSync(file, tarGz(tarEntry("demo", "evil"))),
      error: /checksum mismatch/,
    },
    {
      name: "missing archive",
      change: (file) => fs.rmSync(file),
      error: /ENOENT/,
    },
  ];
  for (const tc of testcases) {
    const root = tempDir(t);
    fs.cpSync(mirrorDir, path.join(root, "mirror"), { recursive: true });
    tc.change(path.join(root, "mirror", name));
    const dir = path.join(root, "pkg");
    await assert.rejects(
      installer.install(
        dir,
        mirrorEnv(path.join(root, "mirror")),
        "linux",
        "x64",
      ),
      tc.error,
      tc.name,
    );
    assert.equal(
      fs.existsSync(installer.binaryPath(dir, "linux")),
      false,
      `${tc.name}: binary exists`,
    );
  }
});

test(
  "the launcher runs the installed binary and explains a missing one",
  { skip: process.platform === "win32" && "needs a shebang binary" },
  (t) => {
    const dir = path.join(fs.realpathSync(tempDir(t)), "pkg");
    fs.cpSync(pkgDir, dir, { recursive: true });
    const launcher = path.join(dir, "launcher.js");

    const missing = spawnSync(process.execPath, [launcher], {
      encoding: "utf8",
    });
    assert.equal(missing.status, 2);
    assert.match(missing.stderr, /^demo: the binary is not installed\./);
    assert.ok(
      missing.stderr.includes(installer.fixCommand(dir)),
      missing.stderr,
    );

    const binary = installer.binaryPath(dir);
    fs.mkdirSync(path.dirname(binary), { recursive: true });
    fs.writeFileSync(
      binary,
      `#!${process.execPath}\nconsole.log(process.argv.slice(2).join('|'));process.exit(3);\n`,
      { mode: 0o755 },
    );
    const run = spawnSync(process.execPath, [launcher, "a", "b c"], {
      encoding: "utf8",
    });
    assert.equal(run.status, 3);
    assert.equal(run.stdout, "a|b c\n");
  },
);

test("a failed install names the base URL override for users behind a proxy", (t) => {
  const dir = path.join(tempDir(t), "pkg");
  fs.cpSync(pkgDir, dir, { recursive: true });
  const empty = path.join(tempDir(t), "empty");
  fs.mkdirSync(empty);

  const run = spawnSync(process.execPath, [path.join(dir, "install.js")], {
    encoding: "utf8",
    env: { ...process.env, ...mirrorEnv(empty) },
  });

  assert.equal(run.status, 1);
  assert.match(run.stderr, /demo: install failed: /);
  assert.match(run.stderr, /Behind a proxy, set DEMO_DOWNLOAD_BASE_URL to a directory URL/);
});
