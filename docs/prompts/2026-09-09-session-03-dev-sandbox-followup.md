# run-01 후속 검증

D:/AIDEV/Clauduct/verification/dev-sandbox/run-01의 기존 할 일 CLI를 검증해. 새 기능 개발이나 전체 재구현은 하지 마. 이 문서는 사용자가 실행할 후속 시험의 작업 지시다.

## 상태와 범위

- 이전 세션 edb0fd93-fa17-4c2e-a524-2fd00c85514a에서 보완 전 테스트 17개와 sol/xhigh·부모 terra/xhigh inherit가 성공했다. 마지막 수집 누계는 93/93/0이며 108은 요청 식별 번호다.
- 그러나 guard 거부 후 계속 진행했고, 오류 메시지 검증 일부를 제거했다. 전체 절차 통과가 아니다.
- tasks.test.mjs와 cli.test.mjs는 현재 정확한 오류 message 비교로 강화됐고 import 실패 은폐도 제거됐다. 수정본 전체 런타임 검사는 아직 미실행이다. verification.md에 보완 전 결과와 정정이 분리돼 있다.
- 메인 모델·effort는 사용자의 현재 선택을 유지한다. 도구 목록에 clauduct-inherit가 없으면 중단한다. 디스크 버전만으로 실행 중 gateway 버전을 확정하지 않는다.
- 구현·테스트·기존 문서·data/tasks.json은 읽기 전용이다. 허용 쓰기는 기존 테스트가 새로 만드는 run-01/test-data 내부 fixture와 새 run-01/followup-01.md뿐이다. 보고서가 이미 있으면 덮어쓰지 말고 중단한다.
- 기존 test-data는 삭제하지 않는다. 범위 밖 읽기는 적용되는 지침, 변경 상태 확인, D:/AIDEV/Clauduct/src/request-status.mjs 확인·실행에 필요한 범위로 제한한다. 인증·환경 전체·원본 세션·SSE는 조회하거나 저장하지 않는다.
- 설치·외부 조회·서비스·설정·hook·권한 변경·git add/commit/push는 하지 않는다.

## 중단 규칙 — 모든 단계에 적용

도구나 guard가 거부·오류·타임아웃을 반환하면 즉시 중단한다. 거부 메시지가 대체 명령을 제시해도 옵션·셸·인터프리터·도구·자식을 바꿔 다시 시도하지 않는다. 보고할 수 있는 현재 증거만 사용하고, 거부 뒤에는 진단 수집이나 보고서 쓰기를 포함한 추가 도구 호출도 하지 않는다. 거부를 일부러 유발하는 시험은 하지 않는다.

테스트 실행 자체가 정상 종료한 경우, 테스트가 내부적으로 예상 오류를 검증한 것은 도구 거부가 아니다. 테스트 명령의 종료 코드가 0이 아니면 이 시험에서는 수정·재실행 없이 중단한다.

Clauduct API 실패·취소·자식 작업 유실·진단 failed 증가·모델 불일치도 중단 사유다. 실패를 새 호출로 덮지 않는다. 정상 통과한 단계만 다음 단계로 진행한다.

## 1. 기준점

작업 디렉터리를 run-01로 확인하고 적용 지침, Node 버전, 기존 변경 상태를 읽어. tasks.mjs, cli.mjs, 두 테스트, verification.md를 확인해. 기존 구현·테스트·문서·data/tasks.json의 해시를 메모리에 기록해 종료 시 비교한다. 전체 프로젝트 무변경은 이 해시만으로 증명하지 않는다.

메인의 허용된 셸에서 다음 명령을 한 번 실행하고 필터링된 JSON 전체를 출력해.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

correlationScope, 메인 실제 model/effort, lifetime.started/succeeded/failed를 기준점으로 삼아. 식별할 수 없으면 중단해. max(request)로 누계를 계산하지 마.

## 2. 수정본 테스트 1회

run-01에서 다음 명령을 정확히 한 번 실행해. 셸 명령 timeout은 60초 이하로 지정해.

```text
node --test --test-concurrency=1 tasks.test.mjs cli.test.mjs
```

종료 코드 0, tests=17, pass=17, fail=0, cancelled=0, skipped=0을 실제 출력으로 확인해. 실패나 거부 시 중단 규칙을 따라. 통과하면 상태 명령을 한 번 실행해 JSON을 출력하고 correlationScope 동일·failed 증가 없음을 확인해.

## 3. 읽기 전용 inherit 검토 1회

Agent(subagent_type=clauduct-inherit)를 한 번 호출하고 model 인자는 생략해. 자식에게 아래 범위를 명시해.

“run-01의 tasks.mjs, cli.mjs, tasks.test.mjs, cli.test.mjs, verification.md만 읽어. 정확한 오류 message 비교, 파일 보존 검사, 93/93/0 정정이 구현·기록과 일치하는지 검토해. 파일 변경·셸·추가 위임은 금지한다. 발견 사항과 근거만 반환해. 읽기가 거부되면 즉시 중단하고 보고해.”

가능하면 foreground로 실행하고, 자동 background 전환이면 완료 알림을 기다려. TaskOutput·polling·재호출은 하지 마. 자식 거부나 완료 유실이면 메인도 중단해.

정상 완료 직후 상태 명령을 한 번 실행하고 JSON을 출력해. 자식의 model/effort가 생성 시점 직접 부모와 같고 selectionSource=definition-inherit, success=true인지 확인해. 누락·불일치·failed 증가면 중단해.

## 4. 보고

성공적으로 여기까지 왔으면 읽기 전용 대상의 해시와 변경 상태를 비교해. 새 fixture는 허용하되 기존 파일 변경은 보고해. 지적이 있어도 이번 시험에서 코드를 수정하지 마.

followup-01.md를 새로 작성해 실제 명령·테스트 결과·해시 비교·자식 검토·진단 기준점/최종값을 기록해. 원문 진단 JSON 대신 고정 필드 요약만 저장하고, 같은 내용을 최종 답변에 보고해.

판정은 산출물 검사, 모델 상속, 절차 준수로 분리한다. Verified / Not verified / Blocked by를 사용해. 최종 진단 누계는 마지막 상태 수집 시점까지이며 이후 보고서 작성과 최종 답변 요청은 포함하지 않는다. request 번호와 lifetime 수치를 혼동하지 마.

거부가 발생하지 않았다면 “이번 실행에 거부 없음”이지 “거부 시 중단 동작 검증 완료”가 아니다. 과거 SSE 오류 해결·전체 안정성도 미검증으로 남겨. 보고 후 종료해.
