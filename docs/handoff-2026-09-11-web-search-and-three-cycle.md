# 인계 — 웹 검색 구현 완료, SDD 무인 3주기 실행 대기

작성 시점의 사실만 적는다. 서술이 아니라 상태다.

## 1. 지금 무엇이 참인가

- branch `fix/native-completion-resume`, HEAD `7ad8b8e`. tracked 변경 없음.
- 웹 검색이 **구현되고 실사용으로 검증됐다.** 이번 세션에서 끝난 일이다.
- SDD 무인 3주기는 **준비 완료, 실행 대기.** 선행 조건 3개가 모두 닫혔고 프롬프트와 샌드박스가 준비돼 있다.
- 감독하 사용 판정은 PASS. 무인 판정은 여전히 HOLD이며 3주기 실행이 그 판정의 대상이다.

## 2. 이번 세션에서 끝난 일 — 웹 검색

### 결론

lite 봉투에 백엔드 내장 검색은 **없다.** 공개 소스(openai/codex `da20788`, 2026-09-11)가 명시적으로 차단한다.

```rust
// "Responses Lite accepts schemas for client-executed tools, not hosted Responses tools."
if model_info.use_responses_lite || is_basic_session_source(...) { return Vec::new(); }
```

기준 클라이언트는 대신 클라이언트 실행 `web.run` 도구를 쓰고, 그 호출을 `POST <base_url>/alpha/search`로 답한다. Clauduct는 같은 엔드포인트를 같은 자격으로 직접 호출한다.

### 구조

Claude Code의 WebSearch side query는 격리된 요청이다 — 유저 메시지 하나(`Perform a web search for the query: <질의>`), 시스템 프롬프트 한 줄, 도구 하나. 게이트웨이가 그것을 탐지해 질의를 뽑고, `alpha/search`에 한 번 POST하고, 응답으로 `server_tool_use` + `web_search_tool_result` + `text` 블록을 만들어 돌려준다. **모델 추론 턴이 없다. 토큰 0, HTTP 1회.**

클라이언트는 결과 블록에서 `title`과 `url`만 읽고 나머지는 주변 `text` 블록에서 가져간다. 그래서 `alpha/search`의 `output` 요약이 `text` 블록으로 들어간다.

### 실사용 증거 — 세션 `a6e7f1ad`, 요청 13

```
success=true  failureCategory=null
webSearchRequested=true  webSearchAnswered=true
webSearchCalls=1  webSearchLinks=15  firstContentBlock=server_tool_use
lifetime: started=12 succeeded=12 failed=0, failuresByStage 전부 0
```

링크 15개 반환, 모델이 출처를 인용, 검색 직후 일반 요청도 정상.

### 커밋 (`21177e5`부터 `7ad8b8e`까지)

| 커밋 | 내용 |
|---|---|
| `21177e5` | 프로브에 `--search` 모드. 사용자 SEND 1회로 엔드포인트 확인 |
| `2ae78aa` | `src/native-search.mjs` — 탐지·요청·응답→블록 순수 변환 |
| `2d7f3d4` | 게이트웨이 라우팅과 전송 계층 `search()` |
| `0818d6e` | responses-lite 봉투 전면 폐기 (215줄 삭제) |
| `017175a` | 실 전송 계층 루프백 테스트. 취소 signal 버그 1건 발견·수정 |
| `2aa1188` | 감사 문서 |
| `f3c4919` | `contentBlocks`에 `serverToolUse`·`searchResult` 추가 |
| `3936978` | 404→`SEARCH_UNAVAILABLE`, 429/5xx→1회 재시도, 통지 문구 수정 |
| `7ad8b8e` | 기준표 갱신, 선행 조건 3개 닫음 |

### 경계 (건드리지 말 것)

- 탐지는 클라이언트가 실제로 세우는 조건 **전부**가 맞아야 한다. 하나라도 어긋나면 모델 경로로 흘린다. `tool_choice`는 클라이언트가 항상 보내지 않는다 — 라이브 요청이 upstream 단계까지 갔다는 것이 증거다.
- 질의 하나만 나간다. 기준 클라이언트는 대화 꼬리를 함께 보내지만 **우리는 보내지 않는다.**
- 검색 결과는 정의상 공격자가 쓴 웹 콘텐츠다. 길이·형태 검사, 비-http 스킴과 제어문자는 결과 폐기, 링크도 텍스트도 없으면 `SEARCH_RESULTS_EMPTY`로 실패. **조용한 빈 성공은 없다.**
- 상태 응답에는 개수만 나간다. 질의도 결과도 나가지 않으며 canary 테스트가 고정한다.

### 방법론 교훈 — 여섯 번 틀린 단일 원인

모든 캡처를 `model_provider=capture`로 떴다. 그런데 소스의 게이트가 이렇다.

```rust
available: (is_openai() || uses_openai_actor_authorization() || supports_standalone_web_search)
```

`supports_standalone_web_search`는 `#[serde(default)]` → 커스텀 provider에서 false. **찾던 도구를 관측 방법이 지우고 있었다.** "`web_search=live`와 `disabled`가 동일한 요청을 만든다"는 결론도 여기서 나왔다 — 양쪽 다 억제된 상태였다.

관측 장치가 대상을 지우는지 먼저 확인한다. 이것이 이 세션에서 가장 비싸게 배운 것이다.

## 3. 대기 중인 일 — SDD 무인 3주기 (run-03)

### 사용자와 grilling으로 확정한 결정 (2026-09-11)

| | |
|---|---|
| SDD 범위 | **자식 3종 필수** — 구현 / 독립 명세 검토 / 독립 품질 검토. 주기당 3명 × 3주기 = 최대 9명. 자식 없이 메인이 전부 하면 3주기를 마쳐도 PASS가 아니다 |
| 실행 주체 | 사용자가 새 Clauduct 세션 시작 → 프롬프트 **1회** 전달 → **이후 회신하지 않음** |
| 중단 | 시간 상한 없음. guard·권한·사용량·인증 거부 중 하나라도 나오면 즉시 SAFE-STOP |
| 선행조건 1 | 감독하 관측(46b6af24의 실제 실패 1회 + 4속성)과 이름 포착 장치로 **닫음.** 원인 자체는 여전히 미확정이며 이 판정이 그것을 확정하지 않는다 |
| 주제 | JSONL 로그 집계 CLI 3플랜, `verification/dev-sandbox/run-03` |
| 이후 순위 | 자동 압축 강제 확인 / 문서 정리 / 검색 경로 강건성 — 뒤 둘은 이번 세션에서 완료 |

### 준비된 것

- `verification/dev-sandbox/run-03` — 독립 Git 저장소. `.git`만 존재, status·ls-files 비어 있음, HEAD 없음(`git rev-parse --verify HEAD` exit 1이 정상), local identity `Clauduct Verification <clauduct-verification@example.invalid>`.
- `docs/prompts/2026-09-11-session-25-sdd-three-cycle-run-03.md` — 실행 프롬프트. **이 파일을 그대로 1회 전달한다.**

### 이전 두 시도가 깨진 지점

run-01과 run-02 **둘 다 같은 이유로 절차 불합격**이다. 테스트 러너가 guard에 거부된 뒤 중단하지 않고 계속했다. 기능은 완성했지만 판정은 인정되지 않는다.

session-25 프롬프트는 `src/run-node-tests.ps1`을 처음부터 지정해 그 거부가 발생할 상황 자체를 없앤다. guard 우회가 아니라 **위반하지 않는 방법**이다. 테스트 파일이 없을 때 러너가 던지는 `NO_TEST_FILES`도 도구 장애가 아니라는 것을 프롬프트에 적어 뒀다.

## 4. 함께 볼 문서

| 경로 | 무엇 |
|---|---|
| `docs/remaining-verification.md` | **현행 기준표.** 2~5장이 현재 기준이고 6장은 이력이다. 상태 정의(완료/조건부/미검증/차단/범위밖)가 1장에 있다 |
| `docs/prompts/2026-09-11-session-25-sdd-three-cycle-run-03.md` | run-03 실행 프롬프트. 1회 전달 대상 |
| `docs/audit-2026-09-11-web-search-bridge.md` | 웹 검색 전체 경과. 여섯 번의 실패한 가설과 각각이 왜 틀렸는지 포함 |
| `docs/audit-2026-09-11-live-session-verification.md` | 감독하 PASS의 실사용 증거 |
| `docs/native-feature-support.md` | Claude 네이티브 기능 전수 대조 |
| `src/native-search.mjs` | 검색 탐지·변환. 경계 주석이 설계 근거다 |
| `verification/manual-http-probe.mjs` | 사용자 조작 프로브. `--live`(연결성) / `--live --search`(검색 엔드포인트) |

## 5. 보존할 사용자 상태 — 일괄 stage 금지

`git add -A`나 `git add -- docs`를 쓰지 않는다. 이번 세션에서 두 번 사고가 났다. 아래는 사용자 상태이며 커밋 대상이 아니다.

```
%SystemDrive%/, .tmp/, clauduct-check.txt, clauduct-agent-validation-7b4a674.txt,
docs/handoff-*.md, docs/prompts/*.md, src/agent-selection.review-fixture.mjs,
verification/dev-sandbox/
```

커밋할 때는 변경한 파일을 **하나씩 이름으로** stage한다.

## 6. 검사 기준선 (2026-09-11, Node v24.19.0)

프로젝트 러너로 실행한다.

```powershell
. D:/AIDEV/Clauduct/src/run-node-tests.ps1
Invoke-ClauductNodeTests -Root D:/AIDEV/Clauduct -TestFiles @('src/test-*.mjs')
```

통과: `native-gateway` 61, `native` 47/0, `native-search` 8, `native-transport`, `native-protocol`, `client-version` 12, `request-diagnostics` 69, `upstream-failures` 84, `unsupported-event-diagnostics` 61, `agent-selection`, `completion-selection` 46, `workflow-selection` 36, `request-admission`, `compact-policy`, `file-review`, `launcher-native`, `cancel-snapshot` 6, `verification/test-manual-http-probe` 88.

**환경 때문에 실패하는 4건** — 변경 때문이 아니다. HEAD 워크트리를 따로 떠서 같은 실패를 확인했다.

| 파일 | 이유 |
|---|---|
| `src/test-chat.mjs` (7건) | 이 셸에서 자식 프로세스 spawn 불가 |
| `src/test-review-diff.mjs` | 같은 이유로 git spawn 불가 |
| `verification/test-http-transport.mjs` | 러너의 `--test-concurrency=1`이 프로브의 `checkRuntime`에 걸림. 인수 없이 직접 실행하면 31/31 통과 |
| `verification/test-dotnet-http-transport.mjs` | 같은 이유. 직접 실행하면 통과(45초 소요) |

## 7. 미검증으로 남는 것

| 항목 | 상태 |
|---|---|
| 최초 `UNSUPPORTED_EVENT other/identifier` 원인 | 미검증. 오프라인 분석 소진. 재발 시 종료 JSON의 `unsupportedEventNames`로 이름을 확보한다. **이를 위해 새 시험을 만들지 않는다** |
| 자동 압축 400K/320K 실제 발동 | 미검증. `--verify-auto-compact`는 축소 창(100K) 확인이며 기본값 발동을 증명하지 않는다. 3주기와 **반드시 다른 실행**이어야 한다 |
| 3주기 종료 후 socket·timer·listener 실측 | 미검증 |
| 동적 symlink·junction 검사 | 차단. 다른 셸·경로로 재현하지 않는다 |
| native 비스트리밍 fallback 차단 분기 실행 | 조건부. 실제 스트리밍 오류가 난 세션의 종료 JSON이 필요하다 |
| `alpha/search` 장기 안정성 | 알파 경로다. 사라지면 `SEARCH_UNAVAILABLE`, 모양이 바뀌면 `SEARCH_RESPONSE_SHAPE`로 이름 붙어 실패한다 |

## 8. 하지 않을 것

- 같은 smoke 시험을 사용자에게 반복 요청하지 않는다.
- 실제 Claude를 대신 실행하지 않는다. 전역 설정·hook·권한·인증 저장 파일을 변경하거나 조회하지 않는다.
- guard가 거부한 효과를 다른 셸·인터프리터·도구로 재현하지 않는다.
- 재현 결함이나 측정 근거 없이 리팩토링하지 않는다. 사용자가 범위 밖으로 지정했다.
- 근거 없는 가설로 실사용 시험을 소비하지 않는다. 측정할 수 없으면 측정 장치를 먼저 만든다.
