# run-01 후속 검증 — 단순 사전 확인

이 문서를 끝까지 읽은 뒤 아래 순서만 실행해. 이전 계획 복원, 신규 설계, worktree 준비가 아니라 기존 테스트 1회와 읽기 전용 검토 과제다. 적용되는 상위 지침은 지키되, 선택적 워크플로가 작업 범위를 확장하게 하지 마.

## 범위와 상태

작업 루트는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. 메인 모델·effort는 사용자의 현재 선택을 유지한다. 도구 목록에 clauduct-inherit가 없으면 중단한다.

보완 전 테스트 17개는 이전 세션에서 통과했다. 그 세션의 마지막 수집 누계는 93/93/0이며 108은 요청 식별 번호다. 현재 두 테스트 파일은 정확한 오류 message 비교와 import 실패 은폐 제거로 강화됐지만 수정본 런타임 검사는 아직 미실행이다. verification.md의 과거 결과를 수정본 통과로 재사용하지 마.

직전 후속 세션은 상태 확인 명령이 .git 안으로 이동해 실패했고 테스트는 실행하지 못했다. 이번에는 cwd를 변경하지 않는다. worktree·브랜치 생성/전환, .git 내부 이동/조사, 이전 세션·계획 검색은 하지 마.

구현·테스트·기존 문서·data/tasks.json은 읽기 전용이다. 허용 쓰기는 테스트가 새로 만드는 run-01/test-data 내부 fixture와 새 run-01/followup-01.md뿐이다. 기존 fixture는 삭제하지 마. 보고서가 이미 있으면 덮어쓰지 말고 중단해. 범위 밖 읽기는 적용되는 지침, 아래 Git 상태 확인, D:/AIDEV/Clauduct/src/request-status.mjs 확인·실행, 이 지시 문서로 제한한다. 설치·외부 조회·서비스·설정·hook·권한 변경·git add/commit/push는 금지한다. 인증·전체 환경·원본 SSE·세션 로그는 조회하거나 저장하지 마.

## 공통 실행·중단 규칙

도구 호출은 순차 실행한다. 각 호출의 결과를 확인한 뒤 다음 호출을 시작한다. 셸 호출 하나에는 명령 하나만 넣고 timeout은 60초 이하로 지정한다. 명령 연결, 명령 치환으로 경로 계산, 오류 무시, 파이프라인으로 종료 코드 은폐는 하지 마.

도구·guard 거부, 오류, 타임아웃, 비정상 종료, Clauduct API 실패·취소, 자식 유실, 진단 failed 증가, 모델 불일치는 즉시 중단한다. exit code가 0이어도 출력에 실제 fatal 오류가 있으면 중단한다. 대체 명령·옵션·셸·도구·자식으로 재시도하지 않는다. 이후 진단이나 보고서 작성도 하지 말고 현재 증거로만 Blocked by를 답한다. guard를 일부러 자극하지 마. 테스트가 내부적으로 예상 오류를 검증하고 테스트 명령은 정상 종료한 경우는 중단 사유가 아니다.

## 1. 고정된 사전 확인

아래 명령을 각각 별도 셸 호출로 한 번씩 순서대로 실행해.

```text
node -p "process.cwd()"
```

결과가 작업 루트가 아니면 이동해서 복구하지 말고 중단한다.

```text
node --version
```

```text
git -C D:/AIDEV/Clauduct status --short
```

기존 변경을 기록하되 복구·정리하지 마. 추가 Git 구조 조사는 필요 없다.

Glob으로 run-01/followup-01.md의 부재를 확인해. Read로 tasks.mjs, cli.mjs, tasks.test.mjs, cli.test.mjs, verification.md를 순차 확인해. 별도 호출로 다음 해시 기준점을 수집해. 이 명령에는 -w를 추가하지 마.

```text
git -C D:/AIDEV/Clauduct/verification/dev-sandbox/run-01 hash-object -- tasks.mjs cli.mjs tasks.test.mjs cli.test.mjs README.md verification.md data/tasks.json
```

이 해시는 지정한 7개 파일의 전후 비교에만 사용한다. 전체 프로젝트 무변경의 증거가 아니다.

허용된 셸에서 다음 상태 명령을 한 번 실행해 필터링된 JSON 전체를 출력한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

correlationScope, 메인 실제 model/effort, lifetime.started/succeeded/failed를 기준점으로 기록해. 필요한 값을 식별하지 못하면 중단한다. 디스크 코드만으로 실행 중 gateway 버전을 확정하지 마.

## 2. 수정본 테스트 1회

```text
node --test --test-concurrency=1 tasks.test.mjs cli.test.mjs
```

정확히 한 번 실행하고 종료 코드 0, tests=17, pass=17, fail=0, cancelled=0, skipped=0을 확인해. 실패하면 수정하거나 재실행하지 마. 통과하면 상태 명령을 한 번 실행해 JSON 전체를 출력하고 scope 동일·failed 증가 없음을 확인해.

## 3. 읽기 전용 inherit 검토 1회

Agent(subagent_type=clauduct-inherit)를 한 번 호출하고 model 인자는 생략해. 자식에게 다음 작업을 전달해.

“D:/AIDEV/Clauduct/verification/dev-sandbox/run-01의 tasks.mjs, cli.mjs, tasks.test.mjs, cli.test.mjs, verification.md만 Read로 순차 검토해. 정확한 오류 message 비교와 파일 보존 검사를 확인하고, 기록이 보완 전 결과와 현재 미검증 상태를 구분하는지 보고해. 93/93/0의 원본 세션은 이번 읽기 범위 밖이므로 독립적으로 재검증했다고 주장하지 마. 파일 변경·셸·추가 위임은 금지한다. 거부·오류가 발생하면 즉시 중단하고 현재 증거만 반환해. 발견 사항과 근거만 보고해.”

가능하면 foreground로 실행해. 자동 background 전환이면 완료 알림을 기다리고 TaskOutput·polling·재호출은 하지 마. 자식 거부·오류·완료 유실이면 메인도 중단한다.

정상 완료 직후 상태 명령을 한 번 실행해 JSON 전체를 출력한다. 자식 model/effort가 생성 시점의 직접 부모와 같고 selectionSource=definition-inherit, success=true인지 확인해. scope 변경·필드 누락·실패 증가는 중단 사유다.

## 4. 보존 확인과 보고

1단계의 hash-object 명령과 git status 명령을 각각 별도 호출로 한 번씩 실행해 전후를 비교한다. 지정 파일 변경이 발견되면 임의 복구 없이 중단해. 자식 지적이 있어도 코드는 수정하지 마.

정상적으로 여기까지 왔을 때만 followup-01.md를 새로 작성해 명령·실제 테스트 결과·7개 파일 해시 비교·자식 검토·진단 기준점과 마지막 누계를 기록해. 원문 진단 JSON은 저장하지 말고 고정 필드 요약만 사용해. 같은 결과를 최종 답변에 보고한다.

산출물 검사, 모델 상속, 절차 준수를 분리하고 Verified / Not verified / Blocked by를 사용해. request 번호와 lifetime 누계를 혼동하지 마. 마지막 누계는 마지막 상태 수집 시점까지이며 이후 해시 확인·보고서 작성·최종 답변 요청을 포함하지 않는다.

거부가 없었다면 “이번 실행에 거부 없음”으로만 기록해. 거부 시 중단 동작, 과거 SSE 오류 해결, 장기·전체 안정성까지 검증했다고 주장하지 마. 보고 후 종료한다.
