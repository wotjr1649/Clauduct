# V2 호환성 — 현재 / 제약 / 미지원

기능별 **현행**을 적는다. 격차의 이력과 근거는 [PARITY.md](PARITY.md), 실행 증거는
[VALIDATION.md](VALIDATION.md)가 소유한다. 여기에 복제하지 않는다.

검증된 조합: **claude 2.1.274 · codex-cli 0.154.0 · Windows 11 10.0.26200 amd64 · go 1.27.1
(CGO_ENABLED=0)**. 2026-09-17 재확인.

## 1. 동작하는 것

| 기능 | 근거 |
|---|---|
| 스트리밍 대화·도구 왕복·도구 결과 재실행 방지 | TOOL01/02/06, 실클라이언트 |
| 이미지 입력 | A1, 실백엔드 왕복 |
| **PDF 입력**(`document` 블록 → `input_file`) | 2026-09-17. 실백엔드가 PDF 안에만 있던 토큰을 돌려줬다. user 첨부와 **tool_result(Read) 양쪽** |
| 대화 중 도구 추가/삭제(`tool_addition`/`tool_removal`) | A4a, 베타 게이트 포함 |
| 추론 왕복(`redacted_thinking` ↔ `reasoning.encrypted_content`) | A4b, 실백엔드가 자기 기록을 되받음 |
| hosted **WebSearch** | A2 + 2026-09-17 실백엔드 재확인(32,060 bytes, 20 links) |
| **구조화 출력**(`output_config.format` → `text.format`) | 2026-09-17. 그 전까지는 검증만 하고 버렸다 |
| MCP 서버(`--mcp-config`) | G5. 서버가 뜨고 툴이 제공되고 **상속 환경이 살아남는다** |
| `--resume` · `--permission-mode` · `--worktree` · `--plugin-dir` · `--bare` | G5, 전부 동작으로 측정 |
| 서브에이전트 역할 라우팅 | hook(`clauduct-hook`)이 role 보고 → Explore/Plan/general-purpose 재지정 |
| 위임 메뉴 14종(`clauduct-<model>-<effort>`) | effort는 게이트웨이가 맡는다(정의로는 지정 불가) |
| 모델 피커 + `GET /v1/models` discovery | A3 + B1 |
| 컨텍스트 400k / 압축 320k / 압축 요청 effort 상한 | B1, E4 |
| 진단(`GET /clauduct/status`)·종료 요약·상태 파일·rate limit 헤더 관찰 | D1–D6 |
| 취소·프로세스 트리 정리·동시 세션 격리 | LIFE·REL 계열 |

## 2. 제약

| 항목 | 내용 |
|---|---|
| 거부하는 native 옵션 **4개** | `--dangerously-skip-permissions`·`--allow-…`(권한), `--settings`·`--setting-sources`(이 런처가 직접 주입하며, 두 번째 `--settings`는 병합이 아니라 대체다) |
| 주입하는 것 | `--settings`(hooks·modelPicker), `--agents`(위임 메뉴), 세션 환경 14키 + 필수 1키. **사용자 환경·사용자 `--agents`가 이긴다** |
| hook이 없으면 | 역할 라우팅과 메뉴의 effort가 동작하지 않는다. 계정의 `hookInstalled`가 매 세션 그것을 말한다 |
| non-streaming 요청 | 거부. 대신 클라이언트의 fallback 자체를 환경변수로 끈다 |
| 미지 SSE 이벤트 | 요청 실패. 이름만 계정에 남긴다(D6) |
| side query가 아닌 hosted 도구 요청 | `HOSTED_TOOL_UNSUPPORTED`로 거부. 기준선은 조용히 성공시킨다 — 의도적 divergence |
| 사전 출력 토큰 상한 | **불가능.** 이 backend는 `max_output_tokens`를 HTTP 400으로 거부한다. 완료 후 usage 검사만 |
| 게이트웨이 재시도 | 0. 연결 실패의 안전한 경우는 Go 표준 라이브러리가 이미 처리한다(PARITY F1) |
| 런타임 의존 | `claude.exe` + `codex.exe`(버전이 요청 헤더) + `~/.codex/auth.json` |

## 3. 미지원

| 항목 | 상태 |
|---|---|
| `POST /v1/messages/count_tokens` | **구현하지 않는다 (2026-09-17 결정).** `-p` 세션에서 클라이언트가 부르지 않았고(요청 4건 전수 관측), 부르더라도 클라이언트에 `count_tokens_unreachable` + 추정치 폴백 경로가 있다. 정직한 구현에는 Codex용 토크나이저가 필요하고, 추정치를 API 답으로 돌려주면 클라이언트가 그것을 정답으로 취급한다 |
| `review-diff` 헬퍼 | 미지원. `/code-review`는 동작하되 경로 탈출 방지·2 MiB 상한 없이 plain claude와 같다 |
| 완료 연결 바인딩 4종 / agent 메타 검증 / workflow 저널 검증 | 보류·미구현. [README.md](README.md) 3장 |
| advisor 도구, Anthropic 서버 의존 베타 7종 | 이 backend에서 성립하지 않는다. advisor는 환경변수로 끈다 |
| 비Windows | 없다. 이식이 아니라 새 설계다 |

`web_search` 외의 hosted 도구(`web_fetch`·`code_execution`·`computer`·`text_editor`·`memory`)는
**설치된 클라이언트가 보내지 않는다** — 2.1.274 바이너리에 그 타입 이름이 없다(2026-09-17 실측).
API에는 있으나 이 조합에서는 도달 경로가 없다.

## 4. 제3자 구현이라는 사실

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 **공식 지원하지 않는다고 명시**한다.
V2는 제3자 호환 구현이며 "Anthropic 공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않는다.
지원 조합을 고정해 기록하고 drift 진단을 남긴다 — 클라이언트 버전은 고정하지 않고 **보고**한다.
