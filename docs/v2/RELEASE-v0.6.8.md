# v0.6.8 — 요청 값 검증, 빈 tool 결과, 분류기 압축, native 2.1.288 위험 판정

2026-10-04. 범위는 [#257](https://github.com/wotjr1649/Clauduct/issues/257)이다. 필드별 처리와 판정 표는
[COMPATIBILITY.md](COMPATIBILITY.md)의 2절 "Anthropic 요청 필드의 실제 처리"와 3절 v0.6.8 절을 따른다.

## 바뀐 것

- **빈 tool 결과**([#252](https://github.com/wotjr1649/Clauduct/issues/252), PR #260)
  - content가 없는 `tool_result`를 보낼 때 `function_call_output`의 `output` 키가 빠져, 실제 backend가 요청 전체를 HTTP 400으로 거부했다.
  - 이제 빈 결과는 `output: ""`로 보낸다.
  - 실측(luna/low, 각 3회): 빈 문자열은 모델이 빈 결과로 읽었다. 빈 목록은 모델이 도구를 다시 불렀다.
- **분류기 transcript 압축**([#255](https://github.com/wotjr1649/Clauduct/issues/255), PR #262)
  - native 2.1.288은 auto 모드 분류기가 transcript를 너무 길다고 보고하면 대화를 압축한다.
  - Clauduct는 분류기 요청의 backend context 초과를 502 `CONTEXT_LENGTH_EXCEEDED`로 답했다. 그래서 native가 분류기 불가로 읽었고 도구 호출이 거부됐다.
  - 이제 분류기 route 요청에 한해 400 `prompt is too long`으로 답한다. 같은 분류기가 성공하기 전에 다시 넘치면 `CONTEXT_COMPACTION_INSUFFICIENT`로 답해 압축은 한 번만 요청된다.
- **요청 값 검증**([#253](https://github.com/wotjr1649/Clauduct/issues/253), PR #258)
  - `thinking`(type별 멤버, `display` 값), `metadata`(`user_id` 512자 이하), `cache_control.scope`(`global`)의 값을 검사한다.
  - 잘못된 값은 이름을 밝혀 거부한다(`THINKING_FIELDS`·`THINKING_DISPLAY`·`METADATA_FIELDS`·`METADATA_VALUE`·`CACHE_VALUE`).
  - 기준은 native 2.1.288이 실제로 보내는 형태다. 과금 없는 loopback으로 전수 수집했고, 바이너리 정적 분석에 없던 `display: "updates"`를 실제 요청에서 찾았다.
- **Workflow 대기 만료 라벨**([#259](https://github.com/wotjr1649/Clauduct/issues/259), PR #261)
  - 부하가 클 때 근거 대기 만료가 다른 라벨로 기록되던 것을 고쳤다. 거부 판단은 같다.
- **문서**([#254](https://github.com/wotjr1649/Clauduct/issues/254), PR #263)
  - Anthropic 요청 필드의 실제 처리를 분류해 적었다: 검증만 하는 것, 보내지 않는 것, 사후 검사(`max_tokens`), Clauduct 쪽 흉내(`stop_sequences`), native ToolSearch로 대체한 것.

## native 2.1.288 위험 판정

v0.6.7이 남긴 세 위험을 측정했다.

| 위험 | 판정 |
|---|---|
| 응답 도중 끊긴 뒤 이어 쓰기 | replay 보호를 통과한다([#250](https://github.com/wotjr1649/Clauduct/issues/250)). `-p`와 그 subagent는 text를 완료까지 보류해 이어 쓰기가 생기지 않는다. TUI subagent에서는 이어 쓰기가 생기고, 실제 backend 7회 중 2회는 보고가 끊긴 지점에서 멈췄다(아래 알려진 제한) |
| 첫 요청이 서버 출력 한도를 기다림 | 관측되지 않았다 |
| 분류기 transcript 압축 | 위 #255로 동작한다 |

그 밖에 thinking만 있는 응답의 재시도는 Clauduct 경로에서 발동하지 않는다. `EMPTY_REPLY` 뒤 native는 다시 요청하지 않는다([#251](https://github.com/wotjr1649/Clauduct/issues/251)).

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.8` → `00f20208962932a977bbe20cd7c1f1f9b5544d5d` |
| `clauduct.exe` | `bbb6e40cd1780fa92cee1ae8ac3ef060109d0f6ed1e149612d6c997363e90f4b` |
| `install.ps1` | `ed4f70af63d46ae5972396c4d868166bc41202e165967f0fc1000693cf19ed68` |
| `uninstall.ps1` | `eb060a28e1f7d4ed30438774871798cb562bfb4dff4c9b53465b243a4ce06613` |
| `SHA256SUMS` | `d8af85ec01fd1bf05bc713cb9fbacf2b95fceacfa2ffb23aaa4ab8418d1334e6` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

`install.ps1`·`uninstall.ps1`의 digest가 v0.6.7과 다른 것은 #248이 추적 파일의 줄 끝을 LF로 정규화했기 때문이다.

## 검증

- **로컬**
  - gofmt·vet·build, 근거 태그 vet 2종, `go test -timeout=120m`과 `-race`(각 22개 패키지) 통과. 문서 인용 검사 통과.
  - 작업마다 독립 리뷰를 받고 지적을 반영했다(#253 1회차 1건과 nit, #255 2회 결함 3건, #254 문서 결함 9건). 마지막 재리뷰에서 결함은 없었다.
- **실제 backend(이 릴리스 바이트)**: 기존 회귀 18개 통과.
  - TUI 표본과 빠른 종료 표본의 첫 실행(`tui-01`, `tui-quick-01`)은 하네스가 옛 ptydrive를 넘겨 시작하지 못했다(과금 0). 현재 ptydrive로 다시 돌린 `tui-02`는 통과했다.
  - 빠른 종료 표본의 두 번째 실행(`tui-quick-02`)은 native 보조 생성 요청이 끝나기 전에 종료돼 생성 취소가 함께 기록됐다(v0.6.7 설계대로 상태 출력). 세 번째 실행(`tui-quick-03`)에서 통과했다.
- **실제 backend(신규 영향)**
  - 빈 결과의 output 표현 비교. 현재 제품이 보내는 요청도 3/3 정상이었다.
  - 분류기 overflow 주입 뒤 실제 압축·재판정·종료.
  - TUI subagent 이어 쓰기 8회.
- **설치**: PowerShell 7.6.6에서 다음을 모두 통과했다.
  - 새 설치
  - v0.6.7 설치 후 교체·되돌리기
  - 설치본 세션(8회, 실패 0)
  - 발행 후 검증. 처음 실행은 Release의 대상(`target_commitish`)이 `main`으로 만들어져 실패했다. 대상을 태그의 commit으로 고친 뒤 통과했다
  - `install.ps1 -Tag`
  - v0.6.7의 `--update --yes`
  - `.old` 정리
  - 두 번째 `--update` 무변경
- **이 머신의 실제 설치본**: v0.6.7에서 `--update --yes`로 v0.6.8이 됐다. digest 일치, `.old` 정리, 두 번째 `--update` 무변경, 설정 변경 없음.

## 하지 않은 것과 다시 볼 조건

외부 감사에서 나온 제안 중 이번에 하지 않은 것이다. 판단 근거는 [#257](https://github.com/wotjr1649/Clauduct/issues/257)에 있다.

| 제안 | 하지 않은 이유 | 다시 볼 조건 |
|---|---|---|
| `metadata`를 거부 | native가 모든 `/v1/messages` 요청에 `metadata`를 보내므로 거부하면 세션이 깨진다 | — |
| `budget_tokens`를 거부 | 공개 API에서 `thinking.type: enabled`의 필수 멤버라, 거부하면 그 형태 전체를 받지 못한다. 측정한 native 경로는 보내지 않았다 | — |
| `budget_tokens < max_tokens` 교차 검사 | budget이 backend에 정확히 대응하지 않고, `/context` 계수 요청의 `max_tokens`에는 생성 의미가 없다 | — |
| 함수 도구의 `strict`·`allowed_callers`·`input_examples`·`eager_input_streaming` 지원 | native 2.1.288 경로가 보내지 않는다. `strict`를 정확히 옮길 수 있는 범위는 Anthropic과 OpenAI 스키마 제약이 겹치는 부분뿐이다 | native가 보내기 시작해 거부가 관측될 때 |
| backend의 `cache_write_tokens`를 `cache_creation_input_tokens`로 연결 | backend 값의 의미가 확인되지 않았다 | 실측에서 0이 아닌 값이 관측될 때 |
| refusal을 stop_reason으로 연결 | 측정한 경로에서 도달하지 않는다. 만나면 지금처럼 거부한다 | 실제 세션에서 관측될 때 |
| `pause_turn`을 stop_reason으로 연결 | backend 경로에 대응하는 종료 사유가 없다(COMPATIBILITY 2절) | backend가 서버 도구 반복의 중단을 알리는 신호를 낼 때 |
| OpenAI `tool_search` 실측 | native ToolSearch 경로가 동작한다 | 그 경로가 깨지거나 성능·캐시 문제가 생길 때 |
| 매일 도는 유료 drift 실측 | beta 기록과 처음 보는 이름 진단으로 대신한다 | — |
| provider IR 분리 | 다중 provider 계획이 없다 | 그런 계획이 생길 때 |

## 알려진 제한

- TUI subagent의 응답이 도중에 끊기면 native가 이어서 요청하는데, 이어진 보고가 이미 전달된 부분에서 멈출 수 있다(실제 backend 7회 중 2회). 방침은 [#264](https://github.com/wotjr1649/Clauduct/issues/264)에서 정한다.
- hosted web search 도구의 `max_uses`·`allowed_callers`·`response_inclusion`·`cache_control`은 키만 받고 값을 검사하지 않는다. native 2.1.288은 `max_uses`만 보낸다.
- 공개 protocol contract test 정책은 [#256](https://github.com/wotjr1649/Clauduct/issues/256)에서 결정을 기다린다.
- v0.6.7 이전의 알려진 제한은 그대로다([v0.6.7](RELEASE-v0.6.7.md#알려진-제한)). 단 v0.6.7이 남긴 native 2.1.288 위험 셋은 위에서 판정했다.
