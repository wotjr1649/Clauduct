# 정상 종료의 keep-alive 정리 경쟁 상태

## 근거와 한계

사용자가 제공한 session `408352e7-cca9-41b4-b47e-a94da22fdf53`의 종료 자료는 모델 요청 4건 성공, 실패 0건과 `CLEANUP_FAILED`를 함께 기록했다. 요청 성공과 종료 자원 정리는 별개다. 당시 JSON에는 정리 항목이 없어 그 실행의 정확한 실패 자원을 소급 확정할 수 없다.

실제 loopback HTTP 요청을 성공시켜 keep-alive 소켓을 남기고 synthetic native child를 정상 종료하는 회귀 검사를 추가했다. 수정 전 `SUCCESS` 기대에 `CLEANUP_FAILED`가 나와 실패했다. transport는 `socket.destroyed`만으로 close 대기를 완료했지만, 소켓 추적 Set은 이후 `close` 이벤트에서 제거됐다.

## 수정

- 기존 socketDone Promise를 재사용해 실제 close 이벤트까지 기다린다. 지연 sleep이나 추적 Set 강제 초기화는 사용하지 않는다.
- launcher 종료 JSON에 기존 자원 정리 조건을 그대로 반영한 고정 boolean `cleanup`을 추가한다. 원문 오류 및 transport 객체 전체는 노출하지 않는다.
- 정리 오류 주입 시 CLEANUP_FAILED 유지와 민감 내용 미출력을 검사한다.

## 검증

2026-09-11, Node.js v24.19.0, 프로젝트의 `Invoke-ClauductNodeTests` 실행기, 60초 제한으로 다음 10개 파일이 모두 통과했다.

`test-cancel-snapshot.mjs`, `test-native.mjs`, `test-native-protocol.mjs`, `test-native-transport.mjs`, `test-native-gateway.mjs`, `test-upstream-failures.mjs`, `test-request-diagnostics.mjs`, `test-launcher-native.mjs`, `test-client-version.mjs`, `test-compact-policy.mjs` (모두 src 아래).

cancel-snapshot 6개 시나리오에는 정상 keep-alive 종료, 반복/동시 close 호출, 실제 실패 보존이 포함된다. native 45개, gateway 43개, upstream failures 84개, request diagnostics 24개 검사도 통과했다. 외부 요청·실제 인증 조회·실제 Claude 실행은 하지 않았다.

## 남은 확인

실제 native 새 프로세스에서 session-21의 기존 최소 읽기/정상 종료 절차를 한 번 재실행한다. 종료 줄, 최종 JSON의 cleanup, 세션 ID로 판정한다. 의도적인 취소나 Bash 진단 호출은 추가하지 않는다. 모든 API 오류 해결, SDD 완주, 장시간 무인 개발 검증으로 확대 해석하지 않는다. run-02의 사용자 미커밋 개발 산출물은 변경하지 않았다.
