# 등록 종료와 활성 요청 취소 연결

## 재현한 공백

기존 SubagentStop과 동일 ID 재등록은 selectionController를 취소하고 등록을 제거/교체했다. 그러나 이미 선택이 확정된 요청은 별도의 요청 controller로 transport와 delivery를 실행하므로 등록 종료가 그 요청에 전달되지 않았다. 선택을 캐시한 자식의 활성 요청 2개를 만든 loopback 검사에서 stop/re-registration must abort active child transports assertion 실패를 확인했다.

실제 사용자 세션에서 이 경쟁 조건이 발생했다는 증거는 아니다. native의 자체 HTTP 연결 종료는 별도로 취소를 전달할 수 있지만, gateway가 받은 등록 종료만으로도 이전 등록의 실행 중 요청을 취소해야 한다.

## 변경

등록 객체에 해당 등록의 활성 요청 controller Set을 둔다. 최초 수정은 본문을 읽고 선택 단계에 진입한 요청만 연결했다. 아래 후속 수정에서는 admission 이전으로 연결을 옮겼으며 모든 종료 경로의 finally에서 제거한다. 등록 종료/교체 시 기존 선택 대기와 이 Set의 요청을 취소한다. 역할 충돌은 취소 전에 거부한다. 새 등록의 요청, 형제 등록, 메인 요청에는 취소를 전파하지 않는다.

취소된 transport가 늦게 정상 이벤트 배열을 반환하더라도 gateway가 완료 처리 전에 signal을 재확인한다. 정상 완료·새 생성 호출의 근거로 사용하지 않는다. 기존 request controller를 사용하므로 transport의 retry 중단 및 delivery 취소 경로를 재사용한다. 전역 상태/새 의존성/권한·hook 변경은 없다.

## Verified

test-native-gateway에 stop/재등록 × 응답 전/스트리밍 중 4개 검사를 추가했다. 실제 gateway·선택기·loopback HTTP와 합성 metadata/transport를 사용한다.

- 캐시된 선택을 사용하는 동일 자식의 활성 요청 2개 모두 취소.
- 동시에 실행 중인 형제 요청의 signal은 유지되고 정상 반환. 메인도 정상 반환.
- 잘못된 역할의 stop/재등록은 400으로 거부되고 활성 요청은 취소되지 않음.
- 응답 전 취소는 502/CANCELLED, 이미 스트리밍한 응답은 연결 종료. 둘 모두 success=false 및 CANCELLED 진단.
- 취소 후 이벤트를 반환하는 legacy transport도 성공 처리하지 않음.
- 요청 종료 후 admission.active와 activeDeliveries가 0으로 복귀.

Node --permission에서 아래 검사를 실행했다. 실제 Claude·외부 요청은 0이다. 파일 fixture 검사는 src 쓰기만 허용하고 자체 정리하며 실제 인증 경로는 읽지 않는다.

```text
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-transport.mjs
```

결과: native-gateway 39, agent-selection passed=true, completion-selection 46, workflow-selection 36, native-transport passed=true. 이전 정상 상속·부모 완료 복귀 로컬 검사는 유지됐다.

## Not verified / Blocked by

최초 수정은 본문 수신/메모리 admission 대기 구간을 포함하지 않았다. 그 구간의 후속 검증은 아래를 따른다. 실제 native 중단 UI/정상 종료 hook 발행 순서, 원격 backend 계산 자체의 즉시 중단, 전체 취소 조합 및 symlink/junction 동적 검사는 미검증이다. 전송 취소를 이미 전송한 내용의 회수라고 설명하지 않는다.

기존 자동 인증 실행·symlink 거부를 유지했다. 새 사용자 시험은 아직 요청하지 않는다. 실제 UI 시험은 native의 중단·완료 알림 의미를 확인한 뒤 설계한다.

## 후속: admission과 본문 수신 중 등록 고정

수정 전 추가 검사에서 admission 대기 요청이 등록 종료 뒤에도 큐에 남아 expected state transition assertion이 실패했다. 본문 수신 후에야 현재 등록을 조회하던 구조에서는 그 사이 같은 ID가 재등록되면 이전 요청이 새 등록을 사용할 수도 있었다.

요청 header 검증 후, admission 전에 등록 객체를 캡처하고 기존 요청 Set에 연결한다. 본문 수신 뒤 새 등록을 다시 조회하지 않는다. readBody는 동일 controller의 취소를 받아 진행 중인 IncomingMessage를 종료하며, finally에서 abort listener·body timer를 정리한다. 기존 UTF-8/크기 검사와 300초 body timeout을 유지한다. timeout은 REQUEST_TIMEOUT으로 controller를 취소한다. 내부 취소로 소켓이 닫힌 경우 clientDisconnected=true로 잘못 분류하지 않는다.

Verified: admission/body × stop/재등록 4개 loopback 검사를 추가했다. 대기 중 또는 본문 첫 바이트만 보낸 상태에서 등록을 종료/교체한 후, upstream 호출 0, CANCELLED 진단, 큐·activeBodies·admission.active·activeTimers 0을 확인했다. 그 뒤 새 등록의 별도 요청은 200으로 성공했다. 취소된 요청이 pending 생성 근거를 소비하지 않고 새 요청만 정상 선택했다. body 취소는 ECONNRESET, admission 취소는 400/CANCELLED다.

위 다섯 명령을 후속 수정 후 다시 실행했다. native-gateway는 43개 통과이며 agent-selection passed=true, completion-selection 46, workflow-selection 36, native-transport passed=true다. 추가로 다음 검사도 통과했다.

```text
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-request-admission.mjs
```

request-admission은 passed=true, evictionCount=0, queuedAfterClose=0이다. 이 검사는 메모리 부족 시 기존 작업을 쫓아내지 않는 정책도 확인한다. 실제 300초 timeout 경과, native UI의 취소 발생 순서와 원격 backend 중단은 이번 로컬 검증에 포함하지 않는다.
