import test from "node:test";
import assert from "node:assert/strict";

// shared.ts exports are re-exported through the node entrypoint.
import { defineCommand } from "../dist/node.js";

// Import shared internals for direct unit testing. The compiled output
// mirrors src/ structure, so shared.js is available in dist/.
import { deriveEnv, parseCustomCommand } from "../dist/shared.js";

// ---------------------------------------------------------------------------
// deriveEnv
// ---------------------------------------------------------------------------

test("deriveEnv returns undefined when cwd and provided are both empty", () => {
  assert.equal(deriveEnv(undefined, undefined), undefined);
  assert.equal(deriveEnv(undefined, {}), undefined);
});

test("deriveEnv passes through provided env without cwd", () => {
  const env = deriveEnv(undefined, { FOO: "bar" });
  assert.deepEqual(env, { FOO: "bar" });
});

test("deriveEnv derives HOME, USER, LOGNAME, and GROUP from /home/<user> cwd", () => {
  const env = deriveEnv("/home/alice", {});
  assert.equal(env.HOME, "/home/alice");
  assert.equal(env.USER, "alice");
  assert.equal(env.LOGNAME, "alice");
  assert.equal(env.GROUP, "alice");
});

test("deriveEnv does not override already-provided env values", () => {
  const env = deriveEnv("/home/alice", { USER: "bob", LOGNAME: "bob" });
  assert.equal(env.HOME, "/home/alice");
  assert.equal(env.USER, "bob");
  assert.equal(env.LOGNAME, "bob");
});

test("deriveEnv does not set HOME for non-/home/<user> cwd", () => {
  const env = deriveEnv("/tmp", {});
  assert.equal(env, undefined);
});

test("deriveEnv does not set HOME for nested /home paths", () => {
  const env = deriveEnv("/home/alice/projects", {});
  assert.equal(env, undefined);
});

test("deriveEnv does not derive user fields without cwd even if HOME is provided", () => {
  const env = deriveEnv(undefined, { HOME: "/home/charlie" });
  assert.deepEqual(env, { HOME: "/home/charlie" });
});

test("deriveEnv derives user fields from provided HOME when cwd is set", () => {
  const env = deriveEnv("/tmp", { HOME: "/home/charlie" });
  assert.equal(env.USER, "charlie");
  assert.equal(env.LOGNAME, "charlie");
  assert.equal(env.GROUP, "charlie");
});

// ---------------------------------------------------------------------------
// parseCustomCommand
// ---------------------------------------------------------------------------

test("parseCustomCommand returns null for empty input", () => {
  const commands = new Map();
  assert.equal(parseCustomCommand("", commands), null);
  assert.equal(parseCustomCommand("   ", commands), null);
});

test("parseCustomCommand matches registered command and splits args", () => {
  const handler = (args) => ({ stdout: args.join(","), stderr: "", exitCode: 0 });
  const commands = new Map([["greet", handler]]);
  const result = parseCustomCommand("greet alice bob", commands);
  assert.notEqual(result, null);
  assert.equal(result.handler, handler);
  assert.deepEqual(result.args, ["alice", "bob"]);
});

test("parseCustomCommand returns null for unregistered command", () => {
  const commands = new Map([["greet", () => ({ stdout: "", stderr: "", exitCode: 0 })]]);
  assert.equal(parseCustomCommand("unknown arg", commands), null);
});

test("parseCustomCommand returns null when input contains shell control characters", () => {
  const handler = () => ({ stdout: "", stderr: "", exitCode: 0 });
  const commands = new Map([["greet", handler]]);
  assert.equal(parseCustomCommand("greet | cat", commands), null);
  assert.equal(parseCustomCommand("greet; echo", commands), null);
  assert.equal(parseCustomCommand("greet && echo", commands), null);
  assert.equal(parseCustomCommand("greet > file", commands), null);
  assert.equal(parseCustomCommand("greet < file", commands), null);
  assert.equal(parseCustomCommand("greet (sub)", commands), null);
});

test("parseCustomCommand handles single-quoted arguments", () => {
  const handler = (args) => ({ stdout: args.join(","), stderr: "", exitCode: 0 });
  const commands = new Map([["echo", handler]]);
  const result = parseCustomCommand("echo 'hello world' foo", commands);
  assert.deepEqual(result.args, ["hello world", "foo"]);
});

test("parseCustomCommand handles double-quoted arguments", () => {
  const handler = (args) => ({ stdout: args.join(","), stderr: "", exitCode: 0 });
  const commands = new Map([["echo", handler]]);
  const result = parseCustomCommand('echo "hello world" foo', commands);
  assert.deepEqual(result.args, ["hello world", "foo"]);
});

test("parseCustomCommand handles backslash escapes", () => {
  const handler = (args) => ({ stdout: args.join(","), stderr: "", exitCode: 0 });
  const commands = new Map([["echo", handler]]);
  const result = parseCustomCommand("echo hello\\ world", commands);
  assert.deepEqual(result.args, ["hello world"]);
});

test("parseCustomCommand returns null for unterminated quote", () => {
  const commands = new Map([["echo", () => ({ stdout: "", stderr: "", exitCode: 0 })]]);
  assert.equal(parseCustomCommand("echo 'unterminated", commands), null);
  assert.equal(parseCustomCommand('echo "unterminated', commands), null);
});

test("parseCustomCommand returns null for trailing backslash", () => {
  const commands = new Map([["echo", () => ({ stdout: "", stderr: "", exitCode: 0 })]]);
  assert.equal(parseCustomCommand("echo trailing\\", commands), null);
});

test("parseCustomCommand passes zero args for command with no arguments", () => {
  const handler = (args) => ({ stdout: String(args.length), stderr: "", exitCode: 0 });
  const commands = new Map([["status", handler]]);
  const result = parseCustomCommand("status", commands);
  assert.deepEqual(result.args, []);
});

// ---------------------------------------------------------------------------
// defineCommand
// ---------------------------------------------------------------------------

test("defineCommand returns a CustomCommand object", () => {
  const run = () => ({ stdout: "ok", stderr: "", exitCode: 0 });
  const cmd = defineCommand("test-cmd", run);
  assert.equal(cmd.name, "test-cmd");
  assert.equal(cmd.run, run);
});
