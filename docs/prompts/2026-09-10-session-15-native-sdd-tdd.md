# Session-15: native SDD + TDD 실제 개발 시험

## 목적과 현재 상태

SDD는 Subagent-Driven Development다. Clauduct native Agent에서 구현 자식 → 독립 명세 검토 자식 → 독립 품질 검토 자식 → 메인 통합 검증을 실제로 수행한다. 구현 자식은 TDD로 새 기능을 개발한다. 메인의 역할 분장 설명이나 Skill 호출 자체는 자식 실행 증거가 아니다.

사용자는 새 Clauduct 프로세스의 새 세션에서 이 문서를 입력한다. 작업 루트는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02이며 이미 구현된 로컬 작업 관리 CLI를 확장한다. 빈 디렉터리 검사는 하지 않는다. 기존 저장소를 삭제·초기화하거나 worktree를 만들지 않는다.

기준 HEAD는 298a5332e8f4f698002e236b020adf0d803f1562다. 직전 코드 보완 커밋은 4352906이며 기존 36개 테스트가 통과했다. 최초 세 회차 개발은 f4641007-f156-416d-80d4-a01e4be201aa에서 진행됐다. TDD 성과는 있지만 guard 거부 후 계속 진행해 전체 THREE-CYCLE-PASS는 인정하지 않는다. 그 기록은 변경하거나 이번 결과와 합산하지 않는다.

읽을 파일: 루트의 package.json, tasks.mjs, cli.mjs, test/helpers.mjs, test/*.test.mjs, README.md, VERIFICATION.md. 이번 개발과 관련된 범위만 읽고 원본 세션·인증·전역 설정·다른 저장소는 탐색하지 않는다. 유일한 코드·문서 쓰기 대상은 run-02 내부다. 이 문서 자체와 Clauduct 소스는 수정하지 않는다. 모든 작업은 현재 호스트 권한과 지침 안에서 수행한다.

## 사전 확인

Git top-level과 git/common 디렉터리가 run-02 및 run-02/.git인지, HEAD가 위 기준과 일치하고 작업 트리가 clean인지 확인한다. 불일치하면 덮어쓰거나 정리하지 말고 SAFE-STOP한다. Git identity는 기존 local 값을 사용하며 변경하지 않는다.

package.json의 test 명령과 코드·테스트 효과를 확인한 뒤 루트에서 아래 기준선 검증을 60초 제한으로 한 번 실행한다. Node 프로세스와 자식 테스트의 불필요한 인증 환경·외부 전송 경로는 배제하고, 생성·정리는 시험 내부 임시 데이터에 한정한다. 실행 환경을 안전하게 구성할 수 없으면 검증을 수행했다고 주장하지 말고 중단한다.

```text
node --test --test-concurrency=1 test/*.test.mjs
```

36 pass / 0 fail / 0 skip가 기준이다. test runner에 처음부터 동시성 상한을 넣는다. 아래 모든 테스트도 같은 원칙과 60초 제한을 적용한다.

현재 native Agent schema에 clauduct-inherit가 등록되어 있는지 확인한다. 없거나 직접 호출 방식이 불명확하면 SAFE-STOP한다. 임의 --agents, 전역 설정, model alias 변환, 다른 도구를 통한 대체 실행은 하지 않는다. 이번 시험은 native Agent 경로이며 Workflow 별도 검증을 주장하지 않는다.

## 새 기능의 수용 기준

untag <id> <tag> CLI와 untagTask(storePath, id, tag) API를 추가한다.

- 기존 작업에서 정확히 일치하는 태그 하나를 제거하고 갱신된 작업을 반환한다. 태그 비교는 대소문자를 구분한다.
- 이미 없는 태그 제거와 반복 제거는 성공하는 멱등 동작이며 저장 파일 바이트를 변경하지 않는다.
- 다른 태그의 순서, ID, 제목, 상태는 보존한다.
- 빈 태그, 없는 ID, 잘못된 CLI 인수는 오류로 처리한다. CLI 실패는 stdout이 비어 있고 stderr에 이유와 nonzero exit code가 있어야 한다.
- 기존 공통 경로 검사와 안전한 저장 경로를 재사용한다. 루트 밖 저장·Git metadata 접근을 허용하거나 검사를 완화하지 않는다.
- README에 명령을 추가하고 기존 36개 회귀 테스트를 모두 유지한다. 새 의존성·설치·서비스·외부 조회는 필요 없다.

## 실제 자식 실행 순서

세 역할은 서로 다른 native 자식이어야 한다. 현재 schema에 맞는 foreground Agent(subagent_type='clauduct-inherit')를 사용하고 model 인수는 생략한다. inherit는 자식 생성 시점의 직접 부모 모델과 effort를 상속한다. 메인 모델을 astra/max 등으로 고정하거나 시험 중 변경하지 않는다.

한 번에 자식 하나만 실행한다. 메인은 각 자식에게 수용 기준, 파일 범위, 기준 커밋, 테스트·중단 규칙, 필요한 앞선 검토 결과를 전달한다. 무관한 세션·비밀·원본 reasoning을 전달하지 않는다. 자식의 추가 위임은 하지 않는다. 호스트에서 SDD/TDD 스킬 적용을 요구하면 읽고 따르되, 스킬은 권한 확대나 거부 우회의 근거가 아니다.

1. **구현 자식 / S15-implementer**
   - 먼저 test/untag.test.mjs에 API와 실제 CLI 테스트를 작성한다. 새 테스트를 실행해 미구현 기능 때문에 실패하는 RED를 확인한다. guard 거부·timeout·환경 오류를 RED로 세지 않는다.
   - 그다음 tasks.mjs, cli.mjs, README.md를 최소 수정한다. 새 API의 경계 거부 사례는 기존 test/boundaries.test.mjs의 격리 앱 방식으로 추가하여 실제 run-02 밖에 쓰지 않는다.
   - 신규 테스트와 전체 회귀를 실행한다. RED/GREEN 명령·exit code·통과/실패 수, 변경 파일, 미검증 항목을 반환한다. 커밋은 하지 않는다.
   - 쓰기 허용 파일은 test/untag.test.mjs, test/boundaries.test.mjs, tasks.mjs, cli.mjs, README.md뿐이다. 메인이 대신 구현하지 않는다.
2. **명세 검토 자식 / S15-spec-reviewer**
   - 새로운 자식 문맥에서 명세, 실제 diff와 테스트를 읽기 전용으로 검토한다. 구현자의 성공 주장만 인용하지 않는다.
   - 위 각 수용 기준과 실제 코드·테스트의 대응을 확인한다. 누락이나 초과 범위가 있으면 파일·위치·근거를 보고한다. 코드·기록·Git을 변경하거나 테스트를 실행하지 않는다.
3. **품질 검토 자식 / S15-quality-reviewer**
   - 명세 검토가 통과한 뒤 새 자식으로 실행한다. 오류 처리, 기존 데이터 보존, 멱등성, 공통 API 재사용, 경계 검사, 실제 CLI 테스트가 결함을 잡는지 읽기 전용 검토한다.
   - 불필요한 추상화와 테스트 완화 여부를 확인한다. 파일·위치·근거를 포함한 결함 또는 검토 범위를 포함한 통과 결과를 반환한다. 파일 변경·테스트 실행·커밋은 하지 않는다.

검토에서 수정이 필요하면 해당 피드백만 새 구현 자식에게 전달하고, 수정 후 새로운 명세·품질 검토 자식으로 다시 확인한다. 보완 회전은 최대 2회, 전체 자식 생성은 최대 9회다. 결과 없는 자식을 반복 생성하지 않는다. 자식 종료를 확인하지 못하거나 실제 결과가 없으면 SAFE-STOP하고 메인이 역할을 흉내 내어 채우지 않는다. native가 예상과 달리 background handle을 반환하면 이번 절차를 성공으로 세지 말고 상태를 보고하며 임의 polling·resume·재위임하지 않는다.

## 메인의 통합·증거 기록

검토가 모두 통과하면 메인이 실제 최종 diff를 읽고 전체 테스트를 다시 실행한다. 메인은 제품 코드·테스트를 대신 수정하지 않으며 통합 검증, SDD_VERIFICATION.md 기록, 로컬 커밋만 맡는다. 결함을 발견하면 위 보완 한도 내에서 구현 자식에게 돌려준다.

SDD_VERIFICATION.md에는 다음을 기록한다.

- 시작 HEAD, 실제 새 세션 ID/sessionRef와 확인 출처. 알 수 없으면 unknown.
- 각 자식의 실제 tool call ID, native agent ID/agentRef 등 도구가 제공한 식별자, 역할, 생성·반환 순서, 변경 파일, 검토 결과. 자기소개나 역할 이름만으로 자식 실행을 증명하지 않는다.
- 구현 자식이 수행한 TDD RED/GREEN과 메인의 독립 최종 회귀 명령·exit code·test 수. 검토자가 확인한 파일·명세 항목과 발견·해결된 결함.
- 모델·effort는 요청/정의와 실제 route 증거를 구분한다. 확인하지 못한 실제 route는 Not verified로 남긴다. 전체 환경·인증·SSE·reasoning은 저장하지 않는다.
- guard 거부, 실패, 사용자 개입 유무, 실제 관측 시간, 비용·토큰은 확인 가능한 값만 기록한다. 사용량 한도를 우회하거나 비용을 추정해 확정값처럼 쓰지 않는다.

기록과 의도한 파일만 명시적으로 stage하여 로컬 커밋한다. 이전 기록·커밋은 보존하고 push/PR/배포는 하지 않는다. commit 후 git status와 hash를 확인해 최종 보고한다.

## 중단과 최종 판정

guard/권한 거부, 사용자 취소, 사용량·인증 제한, 범위 충돌이 발생하면 SAFE-STOP한다. 거부 후 다른 옵션·도구로 시험을 계속하지 않는다. guard 설정을 변경하지 않는다. 새 근거 없는 동일 목표 3회 실패, 보완 한도 초과는 FAILED다. 정상적인 RED는 실패 처리가 아니라 사전에 정한 기능 검증이며, 실제 실패 이유를 확인한다. 사용자 취소 이후 추가 도구는 호출하지 않는다.

구현 자식의 실제 코드 변경과 TDD 증거, 별도 두 검토 자식의 실제 결과 복귀, 메인 최종 회귀·커밋, 절차 준수가 모두 확인되어야 SDD-TDD-FUNCTIONAL-PASS를 보고한다. 모델·effort 상속의 실제 route를 확인하지 못했다면 별도로 ROUTING-NOT-VERIFIED를 명시한다. 기능 통과와 모델 라우팅 통과를 혼동하지 않는다. 조사 담당자가 이후 세션 원본과 자식 기록을 대조하기 전에는 외부 감사 완료라고 주장하지 않는다.

이것은 하나의 기능을 통한 native SDD 경로 시험이다. 전체 세 회차 장기개발, Workflow, 중단·복구, 모든 모델·보안 경계·제품 전체 완료를 대신하지 않는다. 최종 답변은 Verified / Not verified / Blocked by와 새 커밋 hash, 세션 식별자를 포함한다.
