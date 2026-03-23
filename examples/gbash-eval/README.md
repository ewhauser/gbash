# gbash-eval

`gbash-eval` is a standalone LLM evaluator example for `gbash`. It ports the structure of [`bashkit`](https://github.com/everruns/bashkit)'s [`crates/bashkit-eval`](https://github.com/everruns/bashkit/tree/main/crates/bashkit-eval) into Go so `gbash` can be evaluated as an independent bash and scripted-tool harness.

This example intentionally gives heavy attribution to `bashkit` because the evaluator design, CLI shape, and vendored JSONL datasets all start from that upstream crate. The upstream project is Apache-2.0, and this port was derived from commit `39e733b004d3726076d8a9a7456fa8a9688d7bef`.

## Run

From the repo root:

```bash
export OPENAI_API_KEY=your-api-key
go run ./examples/gbash-eval run \
  --dataset ./examples/gbash-eval/data/smoke-test.jsonl \
  --provider openresponses \
  --model gpt-5-codex \
  --save
```

From the `examples/` module:

```bash
cd examples
export OPENAI_API_KEY=your-api-key
go run ./gbash-eval run \
  --dataset ./gbash-eval/data/smoke-test.jsonl \
  --provider openresponses \
  --model gpt-5-codex \
  --save
```

Anthropic runs use `ANTHROPIC_API_KEY` and `--provider anthropic`. OpenAI Chat Completions and Responses runs both use `OPENAI_API_KEY` with `--provider openai` or `--provider openresponses`.

For the common benchmark pass that persists both Anthropic Opus 4.6 and OpenAI 5.4 results, use the bundled script from the repo root:

```bash
export ANTHROPIC_API_KEY=your-anthropic-key
export OPENAI_API_KEY=your-openai-key
./examples/gbash-eval/run-benchmarks.sh
```

The script always requires those API keys to already be exported, and it saves both runs under `examples/gbash-eval/results/`. You can override `GBASH_EVAL_DATASET`, `GBASH_EVAL_OUTPUT_DIR`, `GBASH_EVAL_ANTHROPIC_MODEL`, `GBASH_EVAL_OPENAI_PROVIDER`, `GBASH_EVAL_OPENAI_MODEL`, and `GBASH_EVAL_MONIKER_PREFIX` if you need a different benchmark configuration.

When run from an interactive terminal, the script prompts you to choose all configured benchmarks, a single configured provider/model pair, or a custom provider and model. Set `GBASH_EVAL_PROMPT=0` to skip the prompt and run the full configured matrix non-interactively.

## Datasets

The vendored datasets are copied from upstream `bashkit-eval` without modification to the JSONL schema:

- `data/eval-tasks.jsonl`
- `data/smoke-test.jsonl`
- `data/scripting-tool/discovery.jsonl`
- `data/scripting-tool/large-output.jsonl`
- `data/scripting-tool/many-tools.jsonl`
- `data/scripting-tool/paginated.jsonl`

Generated reports are written to `examples/gbash-eval/results/` by default. That output directory is ignored in this repository.

## Attribution

The Rust crate at [`crates/bashkit-eval`](https://github.com/everruns/bashkit/tree/main/crates/bashkit-eval) is the direct upstream reference for this example. This repo ports the evaluator behavior into Go and adapts the runtime integration to `gbash`, but it does not vendor the upstream `results/` directory or claim the evaluator design as original work.

See [`PROVENANCE.md`](./PROVENANCE.md) for the exact upstream path, commit, license note, and copied artifact scope.
