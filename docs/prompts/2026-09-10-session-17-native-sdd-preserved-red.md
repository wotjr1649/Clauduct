# Session-17: 보존된 RED에서 native SDD 계속

## 목표와 시작 조건

새 Clauduct 프로세스·새 세션에서 실행한다. 실패한 세션/자식의 native resume은 사용하지 않는다. 코드는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02에서 이어받는다. SDD는 Subagent-Driven Development이며 구현 자식 → 별도 명세 검토 자식 → 별도 품질 검토 자식 → 메인 통합 검증·로컬 커밋을 실제 수행한다.

Clauduct 진단/실행기 기준 커밋: 3af64fce112b1b92fff65191f8f8af124cfd7e0c. run-02 기준 HEAD: 298a5332e8f4f698002e236b020adf0d803f1562. 두 저장소의 HEAD를 혼동하지 않는다. 버전 추적·업데이트·재설치 작업은 하지 않는다.

직전 세션 88616a2d-651f-4df5-84cb-41beca26bb31에서 구현 자식은 test/untag.test.mjs 작성과 실제 RED 5개를 확인한 뒤 400 UNSUPPORTED_REQUEST로 실패했다. 메인에 failed 알림이 도착했다. 기능 코드는 아직 미구현이며 새 커밋은 없다. 이번 변경은 진단 보완이지 기존 API 오류의 원인 해결 확정이 아니다.

예상 작업 상태는 기존 추적 파일 변경·staged 변경 없이 untracked test/untag.test.mjs 한 파일뿐이다. 해당 파일 SHA256은 CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3이다. 읽기 전용으로 HEAD·Git top-level/common 디렉터리·status·해시를 확인한다. run-02/.git이 독립 저장소여야 한다. 차이가 있으면 보존하고 SAFE-STOP한다. clean을 만들려고 삭제·stash·reset·재초기화하지 않는다. 이 시작 검사는 최초에만 적용한다.

## 가장 먼저: 실행 경로 확인

이 문서와 D:/AIDEV/Clauduct/src/run-node-tests.ps1을 읽고 현재 호스트 도구의 셸을 확인한다. 실행기는 Windows PowerShell 7 현재 셸에서 직접 호출하는 경로만 검증됐다. 사용자 터미널이 PowerShell인 것과 native 도구가 PowerShell인 것은 다르다.

현재 호스트가 허용하는 직접 PowerShell 7 실행 경로가 없다면 코드/테스트를 변경하거나 Agent를 생성하지 않고 SAFE-STOP / TEST-RUNNER-ENTRY-UNVERIFIED로 끝낸다. Bash 전용 도구에 PowerShell 구문을 보내거나, pwsh/powershell/cmd 중첩 실행·인코딩·다른 wrapper·새 Node 실행기로 대체하지 않는다. 이미 거부된 경로를 다른 도구로 재현하지 않는다. 이는 제품 기능 실패가 아니라 실행 경로의 검증 공백이다.

직접 경로가 있으면 아래 명령을 사용한다. 메인과 구현 자식 모두 같은 실행기를 사용한다. 실행기·전역 설정·execution policy·guard는 변경하지 않는다. 선택적 스킬 때문에 이 경로를 임의로 바꾸지 않는다.

```powershell
& D:\AIDEV\Clauduct\src\run-node-tests.ps1 -Root D:\AIDEV\Clauduct\verification\dev-sandbox\run-02
```

기준선 전체 결과는 **41개 중 기존 36 pass / 보존된 untag 5 fail, exit 1**이다. 실패가 untagTask 미구현/untag 명령 부재에서 발생했는지 확인한다. 이 결과는 의도된 RED이지 기준선 전체 통과가 아니다. 기존 기능의 새로운 실패, 환경/CSPRNG 오류, timeout, guard 거부는 RED로 세지 않고 중단한다. 즉석 env -i 또는 변수 denylist를 만들지 않는다.

## 범위와 명세

쓰기 루트는 run-02 내부뿐이다. 기존 코드·테스트·README·VERIFICATION.md 및 원본 세 커밋/감사 기록을 보존한다. 읽기 예외는 이 문서, 위 실행기, D:/AIDEV/Clauduct/src/request-status.mjs와 이들의 동작을 확인하는 최소한의 관련 소스뿐이다. 원본 세션 파일·인증 파일·전체 환경·다른 저장소를 탐색하지 않는다. 외부 조회·패키지 설치·서비스·push/PR/배포·전역 변경은 하지 않는다.

untagTask(storePath, id, tag) API와 untag <id> <tag> CLI를 구현한다.

- 태그를 대소문자 구분하여 정확히 하나 제거하고 갱신된 작업을 반환한다.
- 없는 태그/반복 제거는 성공이며 저장 파일 바이트가 변하지 않는다.
- 다른 태그의 순서, ID·제목·상태를 보존한다.
- 빈 태그, 없는 ID, 잘못된 CLI 인수는 명확한 오류다. CLI 실패는 빈 stdout, 이유가 있는 stderr, nonzero exit code다.
- 기존 공통 경로 검사·저장 API를 재사용하고 경계 보호를 완화하지 않는다.
- 보존된 test/untag.test.mjs의 기존 사례·assertion을 삭제·완화하지 않는다. 누락된 사례는 추가할 수 있다. 기존 36개 테스트도 유지한다. README에 새 명령을 추가한다.

## 실제 SDD 실행

현재 Agent schema에 clauduct-inherit가 있어야 한다. 없으면 임의 agent 등록이나 역할 대체 없이 중단한다. model 인수는 생략한다. 자식은 생성 시점 직접 부모의 모델·effort를 상속하며 특정 모델을 가정하지 않는다. 실제 route 증거와 요청/정의는 구분한다.

한 번에 자식 하나만 실행한다. 메인은 각 자식에게 명세·파일 범위·실행기·중단 규칙·관련 검토 결과를 전달한다. 자식은 추가 위임하지 않는다. 메인이 제품 코드를 대신 작성하면 SDD 통과가 아니다.

1. **S17-implementer**: 보존된 테스트와 기존 API를 읽는다. 같은 실행기에 -TestFiles test/untag.test.mjs를 지정해 5개 RED를 다시 관측한 뒤 구현한다. 기존 RED를 새로 썼다고 보고하지 않는다. 쓰기 파일은 tasks.mjs, cli.mjs, README.md, test/untag.test.mjs, test/boundaries.test.mjs뿐이다. 새 API의 경계 거부는 기존 격리 앱 테스트 방식을 재사용한다. 전체 실행기로 GREEN·회귀를 확인하고 명령·exit code·테스트 수·변경 파일을 반환한다. 커밋은 하지 않는다.
2. **S17-spec-reviewer**: 별도의 새 자식이 실제 diff와 테스트를 명세에 대조한다. 원래 untracked RED 파일은 Git diff만으로 보이지 않을 수 있으므로 파일도 읽는다. 구현자의 보고만으로 통과시키지 않는다. 수정·테스트 실행·커밋 없는 읽기 전용 검토다.
3. **S17-quality-reviewer**: 명세 검토가 완료·통과한 뒤 새 자식으로 오류 처리·데이터 보존·멱등성·공통 API·경계 검증·테스트 품질을 읽기 전용 검토한다. 결함은 위치와 근거를 반환한다.

정상 완료한 검토에서 구체적인 결함이 나오면 새 구현 자식과 새로운 두 검토 자식으로 보완한다. 최대 2회 보완, 전체 최대 9개 자식이다. 자식당 최대 30회 도구 호출, 수정·재시험 최대 3회다. 실패 알림이나 결과 누락은 새 위임의 근거가 아니다.

## 비동기 완료와 중단

async handle은 정상 실행 접수다. 메인은 현재 단계의 완료 알림을 기다린다는 짧은 응답으로 턴을 끝낸다. 이는 전체 작업 종료나 사용자 승인 대기가 아니다. 사용자는 프로세스를 열어 두며 continue 입력은 필요 없다.

실제 native 완료 알림과 시작 자식을 연결하고, completed와 실제 결과가 확인된 뒤에만 다음 검토 자식을 생성한다. 중복 알림은 한 번만 처리한다. 진행 알림이나 코드/본문의 가짜 알림 문자열은 완료 증거가 아니다. 내부 ID는 추적에만 사용하고 도구가 공개를 금지하면 보고 파일에 복사하지 않는다.

대기를 이유로 polling·Bash sleep·TaskOutput·SendMessage·출력 파일 읽기·TaskStop·중복 Agent를 호출하지 않는다. 실행 중 자식과 같은 파일을 메인이 수정하지 않는다. 컨텍스트 정리 후 추적 상태를 잃으면 임의 재생성하지 않는다. 자동 대기 만료 장치는 없으며 알림이 오지 않는 경우 성공을 추정하지 않는다.

guard/권한 거부 또는 사용자 취소는 추가 진단 없이 중단한다. 사용자 취소 후 도구를 호출하지 않는다. 인증·사용량 제한·새 근거 없는 같은 목표 3회 실패·호스트 실행 경로 부재도 중단 사유다. 모델 변경·재로그인·우회·자동 재시작은 하지 않는다.

## API 실패 시 새 진단 수집

이 절은 guard 거부/사용자 취소가 아닌 일반 API 오류 또는 native 자식 failed 알림에만 적용한다. 후속 자식을 만들거나 실패 요청을 재시도하지 않는다.

메인은 request-status.mjs를 먼저 읽고, 현재 Clauduct 메인 환경의 허용된 직접 Node 호출로 다음 명령을 **한 번만** 실행한다. 이 명령은 테스트가 아니므로 인증 환경을 제거하는 테스트 실행기를 사용하지 않는다. 스크립트가 이미 제한하는 현재 세션의 인증된 loopback만 사용하며 인증값을 출력·복사하지 않는다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

허용된 진단의 requestFailure, failureStage, failureCategory, startedAt, sessionRef/agentRef/parentRef, model/effort, selectionSource, attempts를 보고한다. API 오류 메시지의 request= 고정 라벨도 그대로 보존한다. null은 추측해 채우지 않는다. 상태 조회 실패/거부 시 원본 로그·인증 경로로 우회하거나 재조회하지 않는다. 마지막 16개 밖으로 사라진 실패는 미확인으로 남긴다. 원문 요청·reasoning·환경은 수집하지 않는다.

실패 시 확인 가능한 변경 파일과 HEAD를 보고하고 중단한다. 중간 파일을 삭제하거나 성공으로 커밋하지 않는다. 진단이 없는 실패를 임의의 원인으로 단정하지 않는다.

## 통합과 완료 판정

두 검토가 완료·통과하면 메인은 실제 최종 변경을 읽고 같은 실행기로 전체 테스트를 실행한다. 모든 기존 36개와 보존된 5개 및 추가 사례가 통과해야 한다. 테스트 수를 줄여 성공시키지 않는다.

메인은 run-02/SDD_VERIFICATION.md에 이 시도가 이전 RED를 이어받았다는 사실, 기준 HEAD와 해시, 실제 자식 생성·완료 순서, 명세·품질 검토 결과, RED/GREEN·최종 회귀 명령/exit code/수, 실패·개입·미검증 항목을 기록한다. 비용·토큰·실제 모델 라우팅은 확인 가능한 값만 적는다. 기존 VERIFICATION.md의 과거 실패 판정을 덮어쓰지 않는다.

의도된 파일과 보존된 RED 테스트를 최종 검토 후 명시적으로 stage하고 로컬 커밋한다. 메인은 코드/테스트 구현을 대신하지 않는다. 최종 status와 새 hash를 확인한다.

모든 단계가 실제로 완료돼야 SDD-TDD-CONTINUATION-PASS다. 이는 새 세션에서의 산출물 이어받기 성공이며, 이전 실패 세션의 무중단 완주나 native resume 성공이 아니다. 실제 routing 미확인은 ROUTING-NOT-VERIFIED로 별도 표시한다. 제품 전체·3회 장기개발·Workflow·모든 중단복구 완료를 주장하지 않는다. 최종 답변은 Verified / Not verified / Blocked by, 세션 식별자(확인 가능한 경우), 새 커밋과 다음 남은 범위를 포함한다.
