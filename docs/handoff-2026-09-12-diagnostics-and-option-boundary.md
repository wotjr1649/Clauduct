# 이관 — 진단 신뢰성과 옵션 경계

## 1. 기대 상태

branch `fix/native-completion-resume`, HEAD `6c263c0`, tracked 변경 없음.

```
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct -TestFiles 'src/test-*.mjs'
```

**20/20, 종료 코드 0.** 이제 빨간 것이 하나도 없으므로 실패가 보이면 그것은 신호다. 예외로 취급해 온 항목은 없다.

현재 검증 상태는 `docs/remaining-verification.md`가 가진다(2~5장이 현재 기준, 6장은 이력). 이 세션이 무엇을 왜 바꿨는지는 `git log 7ad8b8e..HEAD`에 커밋 메시지로 있다.

## 2. 먼저 할 일

**완료 알림 기반 복귀 행을 갈라 닫는다.** 사용자와 grilling으로 정한 것인데 아직 반영하지 않았다. 그 행의 "남은 것"은 절반이 **설계상 미지원**(다중 알림·실패/취소 알림 복귀)이고 절반이 미확정 원인(세션 35985327)이다. 미지원을 측정 항목에서 빼고, 원인은 이벤트 이름 두 건과 같이 복구 불가로 적는다. run-04에서 실패 알림 뒤 올바른 비-재실행이 관측된 것이 근거로 붙는다.

**기록되지 않은 관측이 하나 있다.** 사용자가 UI 취소를 직접 확인했고 오류가 나지 않았다고 말했다. `취소·등록 교체·형제 격리` 행에 아직 안 들어갔다.

## 3. 새로 생긴 장치

다음 세션이 있는 줄 모르면 다시 만들 것들이다.

| | |
|---|---|
| `--verify-fallback blocked\|allowed` | 게이트웨이가 콘텐츠 전달 뒤 upstream `error`를 한 번 주입한다. 인자 없이는 돌지 않고 기본 실행은 주입하지 않는다. **idle timeout을 주입하면 안 된다** — 클라이언트가 그 경우를 별도 항으로 처리해 실험이 오염된다 |
| `.clauduct-status/request-status.jsonl` | 종료 JSON이 stdout과 함께 여기 한 줄씩 쌓인다. 사용자에게 붙여넣기를 요청하지 않는다. 경로는 종료 시 `CLAUDUCT_REQUEST_STATUS_FILE`로도 나온다 |
| `blockedOptions` 30개 | 게이트웨이를 벗어나거나 래퍼 계약을 깨는 Claude 옵션을 거부한다. 근거는 `docs/claude-option-classification.md` |
| `lifetime.unsupportedEventNamesWithheld` | 빈 `unsupportedEventNames`가 "없었다"인지 "못 잡았다"인지 가른다 |
| `lifetime.rejectedCategories` | `rejectedBeforeStart`의 내역. 이전에는 첫 라벨 하나만 남았다 |
| `lifetime.injectedStreamErrors` | 그 실행의 오류가 주입된 것임을 표시한다. **0이 아니면 실제 장애 기록이 아니다** |

`--help`가 이것들을 요약한다. 옵션 집합의 실제 내용은 `src/clauduct.mjs` 상단이 가진다.

## 4. 반복해서 나온 결함 부류

이 세션에서 네 번 나왔고 전부 같은 모양이다. **진단이 자기가 답하려는 질문에 답하지 못한다.**

- 빈 목록이 "없음"과 "못 잡음"을 겸했다
- 97건의 거부에 라벨이 하나뿐이었다
- 테스트 하네스가 실패 이름만 남기고 이유를 버렸다
- 종료 JSON이 stdout에만 있어 스크롤백과 함께 사라졌다

고치기 전에 **그 값이 두 가지 이상을 뜻할 수 있는지** 먼저 본다. 새 진단을 추가할 때도 같은 질문을 한다.

두 가지가 여기서 파생된다.

**항상 빨간 검사는 아무도 읽지 않는다.** `test-chat` 7건이 "이 셸에서 spawn 불가"로 적혀 있었고 그중 하나는 자식을 아예 띄우지 않는다. 라벨이 틀렸는데 매번 빨간 탓에 아무도 열어보지 않았다. 환경 때문에 돌 수 없는 검사는 실패가 아니라 `notRun`으로 보고한다.

**관측 장치가 대상을 지우는지 먼저 확인한다.** `autoCompactWindow` 같은 문자열은 소스와 문서에도 있다. 실행 진단인지 텍스트인지 가르지 않고 인용하면 결론이 뒤집힌다.

## 5. 보존할 사용자 상태

`git add -A`나 디렉터리 단위 stage를 쓰지 않는다. 변경한 파일을 하나씩 이름으로 stage한다. 아래는 커밋 대상이 아니다.

```
%SystemDrive%/, .tmp/, clauduct-check.txt, clauduct-agent-validation-*.txt,
docs/handoff-*.md, docs/prompts/*.md, src/agent-selection.review-fixture.mjs,
verification/dev-sandbox/
```

`.clauduct-status/`와 `.clauduct-profile/`은 `.gitignore`가 덮는다.

## 6. 바닥

- guard가 거부한 효과를 다른 셸·인터프리터·도구로 재현하지 않는다. guard가 스스로 안내한 경로(스크립트 파일로 분리, `pwsh -File`)는 우회가 아니다.
- 전역 설정·hook·권한·인증 저장 파일을 변경하거나 조회하지 않는다.
- 실제 Claude를 대신 실행하지 않는다.
- 재현 결함이나 측정 근거 없이 리팩토링하지 않는다.
- 근거 없는 가설로 실사용 시험을 소비하지 않는다. 측정할 수 없으면 측정 장치를 먼저 만든다.
- 같은 smoke 시험을 사용자에게 반복 요청하지 않는다.
