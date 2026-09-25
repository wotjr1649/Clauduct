# v0.4.4 — 실제 검색 검증과 공유 시도 상한

2026-09-26 출하·실제 설치 확인을 완료했다. 태그 `v0.4.4`는
`90067da16cbcddc00a0dfe543cca460aefc99cfb`이며
[GitHub Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.4.4)는 latest·정식 릴리스다.
v0.4.3에서 일괄 거부하던 검증용 WebSearch를 명시적 허용 계획에서 실행할 수 있도록 보완했다.
일반 세션의 검색 동작은 유지한다. [구현 PR #139](https://github.com/wotjr1649/Clauduct/pull/139).

`CLAUDUCT_VERIFICATION_BUDGET`의 `budget.json`에 `allowSearch:true`를 지정한 경우 검색 최초 시도와
허용된 재시도도 모델 생성·계수와 같은 영속 원장에 예약한다. 예약은 credential 읽기·재조회와 전송보다
먼저 끝난다. 프로세스를 새로 열어도 같은 실행의 총상한을 공유하고, 실패·취소한 예약도 돌려주지 않는다.
생략·false는 종전처럼 검색을 거부한다. null·잘못된 타입·중복 필드는 잘못된 계획으로 거부한다.
형식 버전 1은 유지하며 v0.4.3은 새 필드가 포함된 계획을 전송 전에 거부한다.

검색 예약은 전체 `attempts`에 포함하고 `inferences`와 모델별 횟수에는 포함하지 않는다.
원장의 `Search:true`는 예약, `searchTransport.attempts`는 실제 전송 시도다. 예약 뒤 인증 오류처럼
전송 전에 실패한 경우 두 값은 다를 수 있다. [개발 안내](../../go/README.md#검증용-실호출은-별개의-예산).

수정 전 새 회귀 검사가 실패했고, 수정 후 모델·검색 공유 상한, 재시도 전 차단, 잘못된 계획 거부,
실패 예약 유지와 여러 프로세스의 경쟁을 통과했다. 개발 후보의 실제 native WebSearch는
모델 2회·검색 1회, 총 3/8회로 성공했다. 오류·활성 요청·메모리 예약 없이 종료·정리됐다.
수정 후 일반·race 전체 각 21개 패키지와 gofmt·기본/policy_evidence/runtime_evidence vet·build가
통과했다. gated 검사는 SKIP으로 별도 기록하며 전체 통과 수에 합치지 않았다. 메시지 기반 SDK 대기와
background stop→attach 재연결도 무료 native로 다시 확인했다. PR과 병합 commit의 공개 CI도 통과했다.

순수 태그 바이너리의 실제 backend 검증은 Claude Code 2.1.282 / Codex CLI 0.157.0에서 수행했다.

| 실행 | 예약 / 강제 상한 | 결과 |
|---|---|---|
| native WebSearch | 3 / 8 | 모델 2회 + 실제 검색 1회, 실제 검색 결과 URL과 공유 예약 대조 |
| SDK 입력·`/clear`·자식 위임 | 5 / 6 | Luna/max 부모·Luna/low 자식, 결과 회수·종료 |
| 실제 TUI 입력·압축·취소·복구·종료 | 8 / 10 | 같은 세션의 사실·순서 보존, 의도한 취소 1회, 후속 응답 |

모두 API 오류·cleanup 실패 0, 종료 시 활성 요청·메모리 예약·실행·대기 0이다. TUI의 의도한 취소는
성공 요청으로 바꾸지 않고 `CANCELLED`로 확인했다. 순수 출하 검증은 **16회**, 앞선 감사·개발 후보의
실패 실행을 포함한 이번 세션 누계는 **78회**다. 각 실행은 별도 2·6·8·10회 상한을 전송 전에 강제했다.

깨끗한 태그를 Go 1.27.1 windows/amd64·CGO_ENABLED=0·trimpath로 두 번 빌드했으며 독립 빌드 캐시에서도
동일 바이트였다. v0.4.x 규약의 구형 이름 사본 둘도 같은 바이트다.

```text
c6cf11711049be10b442bb46f67127d0339192f0769ebe1b9fd386c456d5c74b
```

격리 새 설치·v0.4.3에서 교체·되돌리기, 공개 태그 고정 설치, v0.4.3의 실제 updater, 다음 실행의 `.old`
정리와 두 번째 업데이트의 no-op을 통과했다. 자산 6개를 다시 내려받아 후보·SHA256SUMS·GitHub API digest를
모두 대조했다. 실제 사용자 설치본도 v0.4.3에서 v0.4.4로 업데이트해 버전·commit·바이트·native 실행을
확인했고, 잔여 `.old`는 없으며 사용자 PATH는 그대로다. 버전 확인 실행의 backend 시도는 0회다.

이번 감사의 원래 v0.4.3 실행에는 Luna의 PDF 식별자 한 글자 누락 1건이 있다. Sol 비교 실행과 무료
native 문서/페이지 이미지 전달 무결성은 통과했다. 모델 판독의 완전성을 주장하거나 이 실패를 삭제하지 않는다.
기존 구현 제약과 다음 기능 묶음은 [호환성 문서](COMPATIBILITY.md#v044--검색-검증의-공유-예산)를 따른다.
