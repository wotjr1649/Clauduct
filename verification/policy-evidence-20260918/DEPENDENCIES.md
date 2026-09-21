# Exact counting dependencies — 2026-09-18

The requested local/backend token counter needs a tokenizer and a WebSocket client.
The previous dependency test explicitly required a recorded decision before adding
dependencies. This is that decision; it does not remove the Node-free runtime guard.

| Module | Pin | Reason |
|---|---|---|
| github.com/tiktoken-go/tokenizer | v0.8.1, ca39f5c7bff9edfc9d70012c5dfdf23b44f7971e | Embedded o200k vocabulary; verified text-only counts without a network request |
| github.com/dlclark/regexp2/v2 | v2.5.1 | Tokenizer's Unicode split expression, which Go regexp cannot represent |
| github.com/coder/websocket | v1.8.15, 9c8faadccd1b679e811a79ce506f8a10237251ad | RFC6455 framing, cancellation and bounded reads for the existing subscription endpoint |
| go.yaml.in/yaml/v3 | v3.0.4, c3552c15f996075a7634df5159d9161c67bf3d76 | Parse native role frontmatter including quoted values, aliases and merge keys without a second handwritten YAML parser |

The module and checksums are pinned. `TestModuleUsesOnlyReviewedPinnedDependencies`
rejects additional modules, version changes, replacements and exclusions. The product
still runs without Node, PowerShell, a vocabulary download or an external count service.
PowerShell is only an independent verification probe.

Focused review covered tokenizer split/merge logic and embedded vocabulary loading,
regexp compilation, WebSocket handshake/redirect handling, reader limits, cancellation
and client options. This is not a full dependency audit. The tokenizer's quadratic
merge work is bounded by refusing runs longer than 512 bytes and text exceeding 4 MiB.
Unsupported local shapes use the verified backend counter where available.

The WebSocket client reuses the project's certificate-verifying, redirect-refusing
HTTP client. Compression is disabled; each event is limited to 1 MiB; at most 64 events
and 30 seconds are allowed. The endpoint is a source constant. Tests cover budget and
synthetic-credential refusal, redirects, missing/invalid usage and unexpected output.

Tokenizer and regexp2 use MIT licences. coder/websocket uses the ISC licence.
Their copyright and licence notices must accompany any redistributed release.
No remote publication or global dependency installation is part of this change.

Role YAML review: Go 1.16 module, no runtime dependencies or installation scripts;
initializers construct reflection/type tables. Decoder alias expansion and parser
depth bounds were inspected. Application reads only a 64 KiB frontmatter prefix,
limits expanded node traversal to 4096 nodes / 32 levels, and rejects cycles and
duplicate routing keys before decoding. This is a focused boundary review, not a
full audit. Both MIT and Apache notices are in THIRD_PARTY_NOTICES.txt.
