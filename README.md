# ARCH-B supplemental materials

Supplemental materials for paper (ARCH-B: Architectural Representation, Comprehension and Hierarchy Benchmark)[https://arxiv.org/abs/2609.34047v1]

This repository contains the released ARCH-B benchmark and the code required to reproduce benchmark execution and scoring. It does not require the source database or private image storage: images are downloaded from the public URLs embedded in `ARCH-B.json` and verified against their released SHA-256 hashes before preprocessing.

## Repository contents

- `ARCH-B.json` is the released 354-question benchmark.
- `benchmark.schema.json` is the JSON Schema for the released benchmark format.
- `evaluation_metadata.csv` records metadata about the evaluation runs used for the results reported in the paper. It is not model output and is not consumed by the runner.
- `runner/` contains the runner-only Go application.
- `runner/.env.example` lists the standard provider environment variables.
- `CHANGELOG.md` records distribution releases.
- `LICENSE.md` contains the MIT License terms.

## Requirements

- Go 1.24.5, pinned by the `toolchain` directive in `runner/go.mod`.
- Network access to the public image URLs and the selected model provider.
- ImageMagick 7.1.1-47 (`magick`/`convert`) or libjpeg-turbo 3.1.1 (`djpeg` and `cjpeg`) for the fallback path used only when Go cannot decode a non-standard JPEG. Normal JPEG, PNG, GIF, TIFF, and WebP inputs are handled in Go.

Build the runner:

```sh
cd runner
go mod download
go build -o bin/benchmark_runner .
```

## Provider configuration

Copy `runner/.env.example` into your preferred secret-management workflow and export only the key needed for your provider. Do not commit API keys.

| Model prefix | Service | Credential | Endpoint |
| --- | --- | --- | --- |
| `openai:` | OpenAI | `OPENAI_API_KEY` | `https://api.openai.com/v1` |
| `gemini:` | Google Gemini | `GEMINI_API_KEY` | Google GenAI SDK default Gemini API |
| `anthropic-batch:` | Anthropic Message Batches | `ANTHROPIC_API_KEY` | `https://api.anthropic.com/v1` |
| `xai:` | xAI | `XAI_API_KEY` | `https://api.x.ai/v1` |
| `ollama:` | Local Ollama | none | `http://localhost:11434/v1` |
| `amazon:`, `meta:`, `anthropic:`, `mistral:` | Amazon Bedrock | `AWS_BEARER_TOKEN_BEDROCK`; optional `AWS_REGION` | `https://bedrock-runtime.<region>.amazonaws.com/v1` |

All providers use their standard public endpoints and canonical environment variables. Bedrock defaults to `us-east-1` when `AWS_REGION` is unset.

## Run the benchmark

From `runner/`, first verify downloads, hashes, and preprocessing without making model requests:

```sh
./bin/benchmark_runner run \
  --input ../ARCH-B.json \
  --slice :1 \
  --preflight-only
```

Then run and score a model:

```sh
export OPENAI_API_KEY="..."
./bin/benchmark_runner run \
  --input ../ARCH-B.json \
  --models openai:gpt-4.1 \
  --mode answer-and-reason
```

Repeat `--models` to evaluate multiple models. `--mode` accepts `answer-only`, `answer-and-reason`, or `both`; `--slice` accepts forms such as `:10`, `5:10`, and `10:`. The runner writes a detailed CSV and summary for each model plus a `results_MULTIBENCHMARK_*.csv` aggregate in the current directory.

The end-to-end reproducibility command for the complete released benchmark is:

```sh
cd runner && go run . run --input ../ARCH-B.json --models openai:gpt-4.1 --mode answer-and-reason
```

Provider use can incur charges. Run the preflight and a small slice before a full evaluation.

## License

This repository is released under the MIT License. See `LICENSE.md` for the full terms.
