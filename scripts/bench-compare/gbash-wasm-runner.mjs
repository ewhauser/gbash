#!/usr/bin/env node

import * as nodeFS from "node:fs";
import { readFile, readdir } from "node:fs/promises";
import { createRequire } from "node:module";
import * as nodePath from "node:path";
import { performance } from "node:perf_hooks";
import { TextDecoder, TextEncoder } from "node:util";
import { pathToFileURL } from "node:url";
import { webcrypto } from "node:crypto";

const runtimeGlobal = globalThis;

main().catch((err) => {
  process.stderr.write(`gbash-wasm-runner: ${formatError(err)}\n`);
  process.exitCode = err?.exitCode ?? 1;
});

async function main() {
  const opts = parseOptions(process.argv.slice(2));
  const shell = await createShell(opts);
  try {
    const result = await shell.exec(opts.command);
    process.stdout.write(result.stdout ?? "");
    process.stderr.write(result.stderr ?? "");
    process.exitCode = Number.isInteger(result.exitCode) ? result.exitCode : 1;
  } finally {
    shell.dispose();
  }
}

function parseOptions(args) {
  const opts = {
    command: "",
    cwd: "",
    wasmPath: "",
    wasmExecPath: "",
    workspace: "",
  };

  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    switch (arg) {
      case "--workspace":
        opts.workspace = nextValue(args, ++i, arg);
        break;
      case "--cwd":
        opts.cwd = nextValue(args, ++i, arg);
        break;
      case "--wasm":
        opts.wasmPath = nextValue(args, ++i, arg);
        break;
      case "--wasm-exec":
        opts.wasmExecPath = nextValue(args, ++i, arg);
        break;
      case "-c":
      case "--command":
        opts.command = nextValue(args, ++i, arg);
        break;
      default:
        throw usageError(`unexpected argument: ${arg}`);
    }
  }

  if (!opts.command.trim()) {
    throw usageError("missing required -c/--command");
  }
  if (!opts.wasmPath.trim()) {
    throw usageError("missing required --wasm");
  }
  if (!opts.wasmExecPath.trim()) {
    throw usageError("missing required --wasm-exec");
  }
  if (opts.workspace && !opts.cwd) {
    opts.cwd = "/workspace";
  }
  return opts;
}

function nextValue(args, index, flag) {
  if (index >= args.length) {
    throw usageError(`missing value for ${flag}`);
  }
  return args[index];
}

function usageError(message) {
  const err = new Error(message);
  err.exitCode = 2;
  return err;
}

async function createShell(opts) {
  ensureNodeGlobals();
  if (!runtimeGlobal.Go) {
    await import(pathToFileURL(nodePath.resolve(opts.wasmExecPath)).href);
  }
  if (!runtimeGlobal.Go) {
    throw new Error("wasm_exec.js did not define the Go runtime");
  }

  const go = new runtimeGlobal.Go();
  const bytes = await readFile(nodePath.resolve(opts.wasmPath));
  const result = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(result.instance);

  const runtime = await waitForRuntime();
  const cwd = opts.cwd || undefined;
  const files = opts.workspace ? await loadWorkspaceFiles(nodePath.resolve(opts.workspace), opts.cwd) : undefined;
  return runtime.createShell({
    cwd,
    env: deriveEnv(cwd),
    files,
  });
}

function ensureNodeGlobals() {
  runtimeGlobal.require ??= createRequire(import.meta.url);
  runtimeGlobal.fs ??= nodeFS;
  runtimeGlobal.path ??= nodePath;
  runtimeGlobal.performance ??= performance;
  runtimeGlobal.crypto ??= webcrypto;
  runtimeGlobal.TextEncoder ??= TextEncoder;
  runtimeGlobal.TextDecoder ??= TextDecoder;
}

async function waitForRuntime() {
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    if (runtimeGlobal.GBashWasm?.createShell) {
      return runtimeGlobal.GBashWasm;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("gbash.wasm did not register globalThis.GBashWasm");
}

async function loadWorkspaceFiles(root, mountPoint) {
  const files = {};
  await walk(root);
  return files;

  async function walk(dir) {
    const entries = await readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      const abs = nodePath.join(dir, entry.name);
      if (entry.isDirectory()) {
        await walk(abs);
        continue;
      }
      if (!entry.isFile()) {
        continue;
      }
      const rel = nodePath.relative(root, abs).split(nodePath.sep).join("/");
      const virtualPath = rel ? nodePath.posix.join(mountPoint, rel) : mountPoint;
      files[virtualPath] = await readFile(abs, "utf8");
    }
  }
}

function deriveEnv(cwd) {
  if (!cwd) {
    return undefined;
  }
  const match = /^\/home\/([^/]+)/.exec(cwd);
  if (!match) {
    return undefined;
  }
  const user = match[1];
  const home = `/home/${user}`;
  return {
    GROUP: user,
    HOME: home,
    LOGNAME: user,
    USER: user,
  };
}

function formatError(err) {
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return String(err);
}
