# Session-19: 보존된 GREEN 구현의 독립 검토와 통합

## 현재 상태와 완료 기준

새 Clauduct 프로세스·새 세션에서 실행한다. 작업 저장소는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02다. 이전 세션을 native resume하지 않는다. SDD는 Subagent-Driven Development다. 이미 구현된 코드를 보존한 채 새 명세 검토 자식 → 새 품질 검토 자식 → 메인 전체 검증·로컬 커밋을 완료한다. 메인은 제품 코드와 테스트를 대신 구현하지 않는다.

이 프롬프트는 session-18의 초기 RED 조건을 대체한다. 원본 session-18을 다시 실행하지 않는다. 세션 d858ce6b-7d52-4e7e-831f-70dbef98cc72의 구현 자식이 보존된 untag RED 5개 → GREEN 5개 → 전체 41개 통과를 실제 실행했고 completed 알림을 반환했다. 이후 메인 API 요청이 실패하여 명세·품질 검토와 최종 커밋은 시작되지 않았다. 과거 실행을 이번 세션에서 수행한 것으로 기록하지 않는다.

Clauduct의 최초 upstream 실패 이벤트 보존 수정은 로컬 회귀 검증된 상태다. 실제 upstream 실패 사유가 해결됐다는 뜻은 아니다. 버전 차이의 CLI_VERSION_UNVERIFIED는 비차단 경고이며 버전 변경·설치·재로그인은 작업 범위가 아니다.

## 최초 확인

읽기 전용으로 run-02의 Git top-level/common directory·HEAD·status와 아래 파일 해시를 확인한다. 독립 .git, HEAD 298a5332e8f4f698002e236b020adf0d803f1562, staged 변경 없음이 기준이다. tracked 수정은 README.md, cli.mjs, tasks.mjs, test/boundaries.test.mjs 네 파일이며 untracked는 test/untag.test.mjs 하나다. 차이가 있으면 보존하고 SAFE-STOP한다. 초기 확인은 최초 한 번만 적용한다.

| 파일 | SHA256 |
|---|---|
| tasks.mjs | DD8288A092C9E5CBC4CF2B018E4A8AA71C0C0124F6DE73BD9E8B7D8A5103A270 |
| cli.mjs | CCEB8503313150049C1B19248AB15840AD57B8FD4B3DC0BD96F370B672E55E3D |
| README.md | 2AC483F825D5268D4EF30B79B5257D848005A0A89BDF42685CC865BCE90BADE2 |
| test/boundaries.test.mjs | DC3B6ACE0EF03FC4731F0EEE09C1B9B0AAFECA5D7768204F3BD37D4EE27EFB1A |
| test/untag.test.mjs | CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3 |

이 문서와 D:/AIDEV/Clauduct/src/run-node-tests.ps1을 읽고 native Bash에서 다음 명령을 실행한다. 기대 기준선은 41 pass / 0 fail / exit 0이다. 다시 RED를 만들거나 구현 자식을 처음부터 생성하지 않는다.

```bash
pwsh -NoProfile -NonInteractive -File D:/AIDEV/Clauduct/src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct/verification/dev-sandbox/run-02
```

도구 timeout은 60초 이상, 실행기 자체 기본 제한은 60초다. 전체 테스트는 항상 이 진입점을 사용한다. 환경/PATH를 임의로 바꾸지 않는다. pwsh 미발견·guard 거부·환경 오류·예상 밖 테스트 실패는 중단 사유다. 다른 shell/interpreter/wrapper로 우회하지 않는다.

## 검토 명세와 범위

쓰기 루트는 run-02뿐이다. 읽기 예외는 이 문서, 위 실행기, D:/AIDEV/Clauduct/src/request-status.mjs 및 이들의 동작 확인에 필요한 최소 관련 소스다. 세션 원문·인증 파일·전체 환경은 열람하지 않는다. 외부 조회·패키지 설치·전역 변경·push/PR/배포는 하지 않는다. 기존 VERIFICATION.md와 기존 테스트/커밋을 보존한다.

untagTask(storePath, id, tag)와 untag <id> <tag> CLI를 다음 기준으로 검토한다.

- 대소문자가 정확히 일치하는 태그 하나를 제거하고 갱신된 작업을 반환한다.
- 없는 태그·반복 제거는 성공하며 저장 파일 바이트가 변하지 않는다.
- 나머지 태그 순서, ID·제목·상태를 보존한다.
- 빈 태그·없는 ID·잘못된 CLI 인수는 명확히 실패한다. CLI 실패는 빈 stdout, 이유가 있는 stderr, nonzero exit다.
- 공통 저장/경로 검사를 재사용하고 외부 경로 거부를 유지한다. 기존 assertion 삭제·완화 없이 README 명령과 경계 테스트가 맞아야 한다.

## 순차 SDD

현재 Agent schema의 clauduct-inherit를 사용하고 model 인수를 생략한다. 자식은 생성 시점 직접 부모의 모델·effort를 상속한다. 특정 모델을 하드코딩하지 않는다. schema에 없으면 임의 등록이나 대체 없이 중단한다. 한 번에 자식 하나, 자식의 재위임은 금지한다.

1. 새 S19-spec-reviewer: 메인이 명세·범위·중단 규칙을 전달한다. 자식은 실제 diff와 전체 관련 소스, Git diff에 나타나지 않는 untracked test/untag.test.mjs를 읽어 명세 충족 여부를 보고한다. 읽기 전용이며 수정·테스트 실행·커밋하지 않는다.
2. 명세 검토가 completed 결과로 통과하면 새 S19-quality-reviewer: 오류 처리·멱등성·데이터 보존·경계 보호·공통 API·테스트 품질을 읽기 전용 검토한다.
3. 정상 완료한 검토가 구체적인 결함을 보고했을 때만 새 구현 자식으로 보완한다. 허용 쓰기 파일은 tasks.mjs, cli.mjs, README.md, test/untag.test.mjs, test/boundaries.test.mjs다. 재현 검사와 최소 수정·전체 회귀 후 새 두 검토 자식으로 다시 검토한다. 최대 2회 보완, 전체 최대 8개 자식, 자식당 최대 30회 도구 호출·수정/재시험 3회다. 실패 알림이나 결과 누락은 재위임 근거가 아니다.
4. 두 검토가 통과하면 메인이 최종 diff와 새 파일을 읽고 같은 실행기로 전체 테스트를 실행한다. 기존 41개 및 추가 사례가 모두 통과해야 한다.

## 비동기와 실패

async handle은 접수다. 메인은 해당 자식의 native 완료 알림을 기다린다는 짧은 응답으로 턴을 끝낸다. 사용자의 continue 입력은 필요 없다. completed와 실제 결과를 확인해야 다음 단계로 간다. 중복 완료 알림은 한 번만 처리한다. 내부 ID는 추적용이며 공개 금지된 값은 보고에 복사하지 않는다.

대기를 이유로 polling·Bash sleep·TaskOutput·SendMessage·출력 파일 읽기·TaskStop·중복 Agent를 호출하지 않는다. 메인은 실행 중 자식 파일을 수정하지 않는다. 결과 없는 실패·인증/사용량 제한·사용자 취소·guard 거부·새 근거 없는 같은 목표 3회 실패 시 중단한다. 취소/거부 후 추가 도구 호출이나 진단 우회는 하지 않는다.

일반 API 오류/failed 알림에는 후속 자식·재시도 없이 중단한다. 메인이 응답 가능한 경우 request-status.mjs를 읽고 현재 Clauduct 환경에서 다음 진단을 한 번만 실행한다. 테스트 실행기로 인증 환경을 제거하지 않으며, 스크립트가 제한한 현재 세션 loopback만 사용한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

failureCategory, upstreamFailureEvent, failureStage, requestFailure, attempts의 terminalState/postCompletionFrame/postCompletionSequence, model/effort/selectionSource를 보고한다. null은 미확인이다. 원문 upstream 메시지·reasoning·인증·환경을 수집하지 않는다. 조회 실패 시 우회/재조회하지 않는다.

메인 자체가 API 오류로 응답하지 못하면 사용자에게 모델 재요청을 요구하지 않는다. 사용자는 같은 Clauduct 입력창의 Bash 모드에서 `! node D:/AIDEV/Clauduct/src/request-status.mjs`를 한 번 실행해 진단을 별도 검증 세션에 전달할 수 있다. 프로세스가 이미 종료됐으면 새 프로세스로 이전 진단을 복원할 수 있다고 주장하지 않는다.

## 기록과 완료

메인은 run-02/SDD_VERIFICATION.md에 이전 구현의 RED/GREEN은 전달받은 과거 증거라는 사실, 현재 기준선·검토 자식 생성/완료 순서·결함/보완·최종 테스트 명령/exit/count·미검증 항목을 기록한다. 실제 모델 라우팅은 관측된 진단이 있을 때만 확정한다. 클라이언트의 모델 표시나 inherit 설정만으로 직접 부모 연결까지 입증했다고 하지 않는다.

최종 검토 후 위 다섯 파일과 SDD_VERIFICATION.md 중 의도된 변경만 명시적으로 stage하여 run-02에 로컬 커밋한다. unrelated 변경은 포함하지 않는다. 최종 HEAD/status를 확인한다. 실패한 상태를 성공으로 커밋하지 않는다.

완료 판정은 SDD-PRESERVED-GREEN-REVIEW-PASS다. 이는 이전 구현을 새 세션에서 독립 검토·통합한 성공이며 한 세션 무중단 SDD, native resume, 제품 전체 검증 완료가 아니다. 최종 보고는 Verified / Not verified / Blocked by와 새 커밋을 포함한다.
