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

## 실제 native 재시험: PASS

사용자가 제공한 session `0d5d6174-f709-4ac1-9c24-90bbbe4a9be0`의 종료 출력으로 판정했다. 원본 transcript를 이번 기록 작업에서 별도 확인한 것은 아니다.

- `Clauduct 종료: SUCCESS`, cleanup의 9개 항목 모두 true.
- 요청 4/5/6 총 3건 성공, 실패 0건, 모두 HTTP 200/completed, 재시도 없음.
- 실제 model/requestedModel은 gpt-5.6-sol, effort/requestedEffort는 low.
- upstream 및 snapshot mismatch 실패, clientDisconnected가 관측되지 않았다.
- clientVersion 0.154.0 / reference 0.153.4 / unverified. context 설정 증거는 inherited-environment, window/autoCompactWindow 400000, compactPercent 84.21052631578947이다. 실제 압축 발동 증거는 아니다.

정상 요청→native 종료→자원 정리→진단 출력의 단일 실제 시험을 PASS로 종료한다. 같은 읽기 시험을 반복하지 않는다. 과거 정리 실패의 원인 소급 확정, 취소 경로, 모든 API 오류 해결, SDD 완주, 장시간 무인 개발 검증으로 확대 해석하지 않는다.

## SDD 재개 준비

run-02의 HEAD 298a5332e8f4f698002e236b020adf0d803f1562, 독립 .git, staged 변경 없음과 기존 네 tracked 수정/한 untracked 테스트를 확인했다. 다섯 파일 SHA256은 기존 session-19 기준과 모두 일치한다. SDD_VERIFICATION.md는 아직 없다. 이번 작업에서 구현 수정·테스트 재실행·sandbox 커밋은 하지 않았다.

다음은 새 세션에서 보존된 GREEN의 기준선 검사, 독립 명세/품질 자식 검토, 최종 검증과 sandbox 로컬 커밋이다. 새 session-22 프롬프트는 과거 session-19의 오류 후 진단 호출 및 사용자 ! Bash 안내를 제거하고 종료 JSON 수집으로 대체한다. 프롬프트는 docs/prompts 관례에 따라 uncommitted로 남긴다.
