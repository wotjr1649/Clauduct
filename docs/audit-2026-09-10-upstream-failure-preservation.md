# 최초 upstream 실패 이벤트 보존

## 관측과 원인 범위

사용자가 제공한 session d858ce6b-7d52-4e7e-831f-70dbef98cc72의 요청 진단에서 요청 34는 main, gpt-5.6-sol/low, upstream HTTP 200, failureStage=upstream, failureCategory=EVENT_AFTER_COMPLETION이었다. transport terminalState=open, postCompletionFrame/postCompletionSequence=null, clientDisconnected=false, 재시도 없음이었다. 요청 35는 별도의 CANCELLED/clientDisconnected=true이며 같은 오류로 합산하지 않는다.

기존 transport는 response.completed만 완료로 기록했다. 프로토콜 검증기는 response.failed/response.incomplete/error도 completed 상태로 저장하고 계속 읽었으므로, 뒤의 이벤트가 EVENT_AFTER_COMPLETION을 발생시켜 최초 실패 종류를 가릴 수 있었다. 해당 경로는 관측 진단과 일치하지만 실제 요청 34의 최초 이벤트/오류 본문은 수집되지 않았다. 정확한 최초 이벤트나 backend 실패 사유를 확정하지 않는다.

버전 진단은 clientVersion=0.154.0, referenceClientVersion=0.153.4, unverified였다. 제공된 자식 요청은 definition-inherit, gpt-5.6-sol/low로 성공했고 메인 33/34도 같은 모델/effort였다. parentRef=null이므로 직접 부모 연결 전체의 증거는 아니다. 버전 변경이 upstream 실패 원인이라는 증거도 없다.

## 변경

하나의 UPSTREAM_FAILURES 고정 매핑을 transport와 프로토콜 검증기가 재사용한다.

| 최초 실패 이벤트 | 오류 분류 |
|---|---|
| response.failed | UPSTREAM_RESPONSE_FAILED |
| response.incomplete | UPSTREAM_RESPONSE_INCOMPLETE |
| error | UPSTREAM_ERROR_EVENT |

실패 이벤트를 받으면 즉시 거부하고 연결을 정리한다. 최초 오류를 정상 완료 상태에 저장하지 않는다. 프로토콜 검증기의 기존 sticky failed 상태는 이후 push/finish에서도 최초 오류를 유지한다. 응답 시작 전 실패도 고정 분류로 종료한다. transport의 기존 SSE/크기/sequence 검사는 그대로 선행하며, 정상 완료 뒤의 추가 이벤트는 계속 EVENT_AFTER_COMPLETION으로 거부한다.

오류는 자동 retry 대상이 아니다. 이미 전달한 text가 있으면 SSE error로 종료하고 성공 message_stop이나 tool 실행 결과를 만들지 않는다. 아직 전달하지 않았다면 gateway HTTP 502와 고정 분류를 반환한다.

gateway 오류의 event=와 request-status의 upstreamFailureEvent는 고정 매핑에서만 나온다. attempts.terminalState에는 실패 이벤트 종류를 남긴다. status는 문자열 여부·allowlist·failureCategory 일치까지 확인한다. 원문 upstream message/code/reasoning은 새 진단에 저장하지 않는다. 후속 이벤트는 기다리거나 처리하지 않으므로 그 경우 postCompletionFrame/Sequence=null은 의도된 미관측이다. 최초 실패 사유를 조사하려고 스트림을 계속 읽는 새 경로는 만들지 않았다.

## 검증

제한된 Node 프로세스(`node --permission --allow-fs-read=D:/AIDEV/Clauduct`)에서 실행했다. 테스트는 합성 데이터와 127.0.0.1만 사용하며 실제 인증/native/model 요청은 실행하지 않았다.

- RED: 새 테스트의 응답 시작 전 실패 분류 검사가 수정 전 MISSING_RESPONSE_START로 실패했다.
- src/test-upstream-failures.mjs: 51 checks, 42 loopback requests 통과. 최초 실패 3종, 시작 전/부분 text 후 실패, sticky 오류, callback/collected transport, 분할 수신, EOF/후속 completion/DONE/잘못된 JSON, retry 없음, socket 정리, HTTP/SSE 오류와 status 전달 및 원문 비노출을 검사했다. 정상 완료 뒤 실패 이벤트 3종은 transport/프로토콜 모두 계속 거부하며, 실패 이벤트라도 잘못되거나 누락된 sequence는 SEQUENCE_MISMATCH로 먼저 거부한다.
- src/test-native-protocol.mjs: 통과.
- src/test-native-transport.mjs: 통과. 기존 완료 후 프레임 거부·sequence·재시도·취소 검사를 포함한다.
- src/test-native-gateway.mjs: 43 passed. 새 분류 및 악성 진단 객체 제거 포함.
- src/test-request-diagnostics.mjs: 24 passed.
- src/test-client-version.mjs: 통과, loopback 12 requests.
- git diff --check: 통과.

독립 서브에이전트 두 개가 테스트 경계와 진단 전달/비노출 경로를 읽기 전용 검토했다. 필수 구현 결함은 없었고, 제안된 오류 객체 비노출·정상 완료 후 실패 3종·sequence 우선순위 검사를 추가해 위 51개 검사로 확인했다. 서브에이전트의 정적 검토는 테스트 실행 증거와 구분한다.

이 수정은 오류 보존·분류의 로컬 검증이다. 과거 요청의 원인 복원, 실제 upstream 장애 해결, 장기 무인 개발 또는 SDD 전체 완주를 의미하지 않는다. 전체 저장소/인증된 native/장기 soak는 실행하지 않았다.

## 보존과 후속 시험

run-02의 HEAD 298a5332e8f4f698002e236b020adf0d803f1562와 미커밋 구현을 유지한다. README.md/cli.mjs/tasks.mjs/test/boundaries.test.mjs 수정과 untracked test/untag.test.mjs는 이번 커밋에 포함하지 않는다. RED 원본 해시 CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3도 유지한다.

이전 native 구현 자식 로그에서 RED 5→GREEN 5→전체 41 통과와 completed 알림을 확인했지만 두 독립 검토·최종 통합 커밋은 남아 있다. 후속 session-19 프롬프트는 새 프로세스에서 GREEN 산출물 해시/전체 테스트 확인 후 명세 검토부터 시작한다. 기존 session-18 초기 RED 요구를 재적용하지 않는다. 프롬프트는 docs/prompts/2026-09-10-session-19-native-sdd-preserved-green-review.md에 별도 미커밋 파일로 둔다.
