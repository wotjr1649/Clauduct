# Clauduct

Claude Code 인터페이스의 모델 요청을 로컬 게이트웨이를 통해 기존 Codex 로그인으로 라우팅합니다.
Read/Edit/Bash/MCP 실행과 사용자 승인은 Claude Code가 그대로 담당합니다.

추론 경로는 **Claude Code → Clauduct의 127.0.0.1 gateway → ChatGPT Codex backend 직접 HTTPS**입니다.
Codex app-server를 호출하거나 실행 중인 Codex 앱 세션에 요청을 넘기는 구현이 아닙니다.

## 구현이 둘입니다

이름이 다르므로 동시에 설치해도 서로를 가리지 않습니다.

| 이름 | 무엇 | 상태 | 현행 문서 |
|---|---|---|---|
| `clauduct` | Go 단일 바이너리 (+`clauduct-hook`, `clauduct-dev`) | **현재 제품** | [docs/v2/README.md](docs/v2/README.md) |
| `clauduct-node` | Node 구현 | 이전 제품 · 비교 기준선 · 되돌리기 경로 | [HANDOFF.md](HANDOFF.md) |

**어느 쪽 문서를 읽고 있는지가 중요합니다.** 이 저장소의 v1 문서는 Node 구현을 기술하며 Go 빌드에
그대로 적용되지 않습니다 — 게이트웨이 재시도 횟수, 자원·선택 실패 코드, Node 런타임 요구,
`--dry-run` 같은 항목이 서로 다릅니다. Go 빌드의 **현행** 동작·제약·미지원은
[docs/v2/COMPATIBILITY.md](docs/v2/COMPATIBILITY.md) 하나가 소유합니다.

## Go 빌드 (`clauduct`)

```powershell
clauduct
clauduct --model sol --effort xhigh
clauduct --continue
clauduct -p "Summarize the current task"
clauduct --update      # Clauduct 자신을 갱신. `clauduct update`는 그대로 전달되어 Claude Code를 갱신
clauduct --usage       # 이 계정이 주간 한도를 얼마나 썼는지. 요청 0회
clauduct-dev doctor    # 이 빌드 자신에 대한 질문은 별도 바이너리로
```

거의 모든 인자는 그대로 native로 전달됩니다. 이 런처가 소유하는 옵션은 `--update`와 `--usage`
둘이고(둘 다 **첫 인자일 때만** 인식), 거부하는 것은 네 개입니다(권한 해제 2종,
`--settings`·`--setting-sources`).

클라이언트의 `/usage`·`/cost`는 GPT 플랜 사용량을 보여주지 못합니다 — 커스텀 base URL에는 계정
엔드포인트를 묻지 않는 것으로 실측됐습니다. `clauduct --usage`가 그 질문에 답합니다.

`claude.exe`와 `codex.exe`, 그리고 `~/.codex/auth.json`이 필요합니다. Node도 .NET도 필요하지
않습니다. 출하물·빌드·되돌리기는 [docs/v2/PACKAGING.md](docs/v2/PACKAGING.md), 모듈의 빌드 명령과
runtime 계약은 [go/README.md](go/README.md)에 있습니다.

## Node 구현 (`clauduct-node`)

Windows 설치는 [설치 안내](docs/installation.md)를 따릅니다. Node.js 24 이상, Claude Code,
Codex CLI와 기존 Codex 로그인이 필요합니다. 동작·검증 구분과 자원 제한은
[native 구현 안내](docs/native.md), 설치·복구·배포 무결성은 [릴리즈 안내](RELEASE.md)에 있습니다.

기준선으로서의 역할이 남아 있어 저장소에 그대로 둡니다. 정리 여부와 그 조건은
[docs/v2/MIGRATION.md](docs/v2/MIGRATION.md) 6장이 기록합니다.

## 무엇이 검증됐는가

증거는 [docs/v2/VALIDATION.md](docs/v2/VALIDATION.md)가 소유합니다. 요구 ID, 게이트, 실행한 검사와
**실행하지 않은 검사**를 함께 적습니다 — 미실행은 통과가 아닙니다.

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 공식 지원하지 않는다고 명시합니다.
이것은 제3자 호환 구현이며 "공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않습니다.
