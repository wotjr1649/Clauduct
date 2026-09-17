# Session-26: run-03 SDD 무인 3주기 실행 진행

## 당신이 무엇인가

일반 Claude Code 세션이며 작업 루트는 `D:/AIDEV/Clauduct`다. Clauduct를 거쳐 실행되는 것이 아니다. 당신은 3주기 시험을 **수행하는 쪽이 아니라 진행시키는 쪽**이다. 시험은 별도의 Clauduct 세션에서 돈다.

## 먼저 읽을 것

1. `D:/AIDEV/Clauduct/docs/handoff-2026-09-11-web-search-and-three-cycle.md` — 인계 문서. 현재 상태, 확정된 결정, 보존할 사용자 상태, 검사 기준선이 전부 여기 있다.
2. `D:/AIDEV/Clauduct/docs/remaining-verification.md` — 현행 기준표. **2~5장이 현재 기준이고 6장은 이력이다.** 6장의 수치와 프롬프트를 현재 설정으로 쓰지 않는다.
3. `D:/AIDEV/Clauduct/docs/prompts/2026-09-11-session-25-sdd-three-cycle-run-03.md` — 실행 프롬프트. 당신이 전달할 대상이다.

읽기 전에 `git status`와 `git log --oneline -5`로 실제 상태를 확인한다. 인계 문서 작성 시점은 branch `fix/native-completion-resume`, HEAD `7ad8b8e`, tracked 변경 없음이었다. 다르면 그 차이를 먼저 확인한다.

## 첫 행동: 사용자에게 두 가지를 요구한다

다른 일을 시작하기 전에 사용자에게 묻는다. 추측하지 않는다.

1. **run-03 시험을 돌릴 Clauduct 실행 명령어.** 모델과 effort를 사용자가 정한다. 형태는 `D:\AIDEV\Clauduct\clauduct.cmd --model <모델> --effort <effort>`다. 자식이 최대 9명이라 이전 시도들보다 비용이 크다는 사실을 알린다 — `luna`가 가장 값싸고 `astra`가 가장 비싸다. `--verify-auto-compact`는 **붙이지 않는다**. 그것은 별개의 시험이고 섞으면 둘 다 판정이 흐려진다.
2. **그 명령으로 시작한 세션의 UUID.**

둘 다 받기 전에는 프롬프트를 전달하지 않는다.

## 두 번째 행동: 프롬프트를 한 번 전달한다

`ListAgents`로 해당 세션을 찾고, `SendMessage`로 `docs/prompts/2026-09-11-session-25-sdd-three-cycle-run-03.md`의 **전문을 그대로** 보낸다. 요약하거나 바꾸지 않는다.

세션 이름이 UUID로는 찾아지지 않을 수 있다. `ListAgents`가 보여주는 이름을 그대로 쓴다.

## 그 다음: 회신하지 않는다

이것이 이 시험의 핵심 조건이다.

전달 후 그 세션이 질문을 보내도 **답하지 않는다.** 답하는 순간 그 실행은 무인 완주가 아니라 회복 시험이 되고 THREE-CYCLE-PASS로 적을 수 없다. 질문이 왔다는 사실만 기록한다. 그 세션은 답이 오지 않으면 가정을 세우거나 SAFE-STOP하도록 프롬프트에 적혀 있다.

사용자가 결과를 가져오면 그때 판정한다. 중간에 진행 상황을 물으러 가지 않는다. `notify_when_idle`로 종료만 기다리는 것은 허용된다 — 그것은 그 세션에 아무것도 전달하지 않는다.

## 결과가 오면 판정한다

그 세션의 최종 보고와 종료 JSON으로 판정한다.

**THREE-CYCLE-PASS 조건 — 전부 충족해야 한다**

- 3개 플랜이 실제 테스트와 로컬 커밋까지 완료됐다.
- 회차마다 구현 자식 1명과 검토 자식 2명이 **실제로 실행**됐다. 자식 없이 메인이 전부 했으면 PASS가 아니다.
- 최초 지시 이후 사용자나 당신의 개입이 없었다.

**PASS가 아닌 경우는 이름을 구분해 적는다**

- `SAFE-STOP` — guard·권한·사용량·인증 거부로 안전하게 멈춤. 안전 정지를 개발 성공과 혼동하지 않는다. 완료 회차와 남은 회차를 명시한다.
- `FAILED` — 검증 실패.

계획된 RED assertion 실패는 **예상된 개발 증거**다. API·환경 오류와 분리해 기록한다. 러너가 실행되어 0이 아닌 종료 코드를 돌려준 것은 도구 거부가 아니다.

자식의 실제 모델·effort가 관측 가능한 증거로 노출되지 않으면 **Not verified**로 적는다. 이름이나 요청값만으로 라우팅 성공을 주장하지 않는다.

판정 결과를 `docs/remaining-verification.md`의 "SDD 무인 3주기" 행에 반영하고, 근거를 새 감사 문서에 남긴다.

## 그 다음 순위

3주기 판정이 끝난 뒤에 착수한다. 사용자가 이 순서로 선택했다.

1. **자동 압축 강제 확인.** `docs/remaining-verification.md` 5.3절 절차를 그대로 따른다. 3주기와 **다른 실행**이어야 한다. 전역 설정을 바꾸지 않으며 `/autocompact` 값을 지정하지 않는다. 이 확인은 축소 창(100K)에서의 발동이며 기본값 400K/320K 발동을 증명하지 않는다 — 그 구분을 결과에 적는다.
2. 3주기가 드러낸 것에 따라 사용자와 다시 정한다.

## 3주기 실행이 함께 채우는 것

이 한 번의 실행이 여러 항목의 증거를 동시에 만든다. 종료 JSON에서 함께 본다.

| 볼 값 | 무엇을 채우는가 |
|---|---|
| `cleanup` 9개, `admission`, `agentRegistrationsEvicted`, `agentRegistrationsExpired` | 장기 자원 안정성 |
| transcript의 `compact_boundary` (`trigger=auto`) | 자동 압축 자연 발동 — 관측되면 기록한다. 채우기용 반복 생성은 하지 않는다 |
| `unsupportedEventNames` | 최초 `UNSUPPORTED_EVENT`의 실제 이름 — 재발했다면 |
| `unknownBetaNames` | beta allowlist 보완 대상 |
| `clientExecutionPolicy.nonStreamingFallbackDisabled`, 스트리밍 오류 행 | native fallback 차단 분기 |
| `webSearchRequested` / `webSearchAnswered` | 검색 탐지가 빗나가지 않았는지 |

`unsupportedEventNames`는 **세션 안에서 조회하면 제거되어 `null`로 나온다.** 반드시 종료 후 launcher가 출력한 `CLAUDUCT_REQUEST_STATUS <JSON>`에서 확인한다.

## 하지 않을 것

- 같은 smoke 시험을 사용자에게 반복 요청하지 않는다.
- 실제 Claude를 대신 실행하지 않는다. 전역 설정·hook·권한·인증 저장 파일을 변경하거나 조회하지 않는다.
- guard가 거부한 효과를 다른 셸·인터프리터·도구로 재현하지 않는다. 거부는 중단 사유다.
- `git add -A`나 디렉터리 단위 stage를 쓰지 않는다. 이전 세션에서 두 번 사용자 파일을 쓸어 담았다. 변경한 파일을 하나씩 이름으로 stage한다.
- 재현 결함이나 측정 근거 없이 리팩토링하지 않는다. 사용자가 범위 밖으로 지정했다.
- 근거 없는 가설로 실사용 시험을 소비하지 않는다. 측정할 수 없으면 측정 장치를 먼저 만든다. 이번 세션의 웹 검색 조사에서 여섯 번의 가설이 하나의 관측 결함 때문에 낭비됐다 — 인계 문서 2장 마지막 절을 읽는다.

## 검증

제품 코드를 바꿨으면 인계 문서 6장의 러너 명령으로 회귀를 돌린다. 거기 적힌 **환경 때문에 실패하는 4건**을 회귀로 오판하지 않는다. 확신이 서지 않으면 HEAD 워크트리를 따로 떠서 같은 실패가 나는지 확인한다.
