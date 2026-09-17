# Session-22: 보존된 GREEN의 SDD 리뷰 재개

## 목표와 현재 상태

새 Clauduct 프로세스·새 세션에서 실행한다. SDD는 Subagent-Driven Development다. D:/AIDEV/Clauduct/verification/dev-sandbox/run-02의 기존 untag 구현을 보존하고 독립 명세 검토 → 독립 품질 검토 → 메인 검증·로컬 커밋을 완료한다. 이전 세션을 resume하거나 session-18/19/21 프롬프트를 추가로 읽지 않는다.

과거 구현 자식의 RED 5개 → GREEN 5개 → 전체 41개 통과는 전달받은 과거 증거다. 이번 세션의 실행으로 기록하지 않는다. 정상 종료 수정 59687cd는 로컬 회귀 10개 파일 통과 후 사용자 세션 0d5d6174-f709-4ac1-9c24-90bbbe4a9be0에서 요청 3건 성공, 종료 SUCCESS, cleanup 9개 true로 확인됐다. 이는 모든 API 오류나 SDD 완주를 검증한 것이 아니다.

## 범위와 최초 확인

쓰기 루트는 run-02뿐이다. 제품·전역 설정·인증·패키지 설치·외부 조회·push·PR·배포는 범위 밖이다. 읽기 예외는 이 프롬프트와 D:/AIDEV/Clauduct/src/run-node-tests.ps1이다. 기존 테스트와 VERIFICATION.md를 보존한다. 상위 지침과 실제 guard를 준수한다.

최초 한 번 Git top-level/common directory, HEAD, status, staged 변경과 아래 해시를 확인한다. 독립 .git, HEAD 298a5332e8f4f698002e236b020adf0d803f1562, staged 없음, tracked 수정 네 파일과 untracked test/untag.test.mjs 하나가 기준이다. SDD_VERIFICATION.md는 아직 없다. 차이는 보존하고 SAFE-STOP한다. 이 기준은 승인된 후속 수정 후에는 다시 적용하지 않는다.

| 파일 | SHA256 |
|---|---|
| tasks.mjs | DD8288A092C9E5CBC4CF2B018E4A8AA71C0C0124F6DE73BD9E8B7D8A5103A270 |
| cli.mjs | CCEB8503313150049C1B19248AB15840AD57B8FD4B3DC0BD96F370B672E55E3D |
| README.md | 2AC483F825D5268D4EF30B79B5257D848005A0A89BDF42685CC865BCE90BADE2 |
| test/boundaries.test.mjs | DC3B6ACE0EF03FC4731F0EEE09C1B9B0AAFECA5D7768204F3BD37D4EE27EFB1A |
| test/untag.test.mjs | CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3 |

실행기를 읽은 후 native Bash에서 아래 명령으로 기준선을 확인한다. 기대는 41 pass / 0 fail / exit 0이다. 전체 테스트는 이 진입점을 사용한다. 도구 timeout은 실행기의 60초 제한보다 여유 있게 설정한다. PATH/환경을 변경하거나 다른 wrapper로 우회하지 않는다.

```bash
pwsh -NoProfile -NonInteractive -File D:/AIDEV/Clauduct/src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct/verification/dev-sandbox/run-02
```

## 검토 명세

untagTask(storePath, id, tag)와 untag <id> <tag> CLI를 검토한다.

- 대소문자 정확히 일치하는 태그를 제거하고 갱신된 작업을 반환한다.
- 없는 태그·반복 제거는 성공하며 저장 파일 바이트를 변경하지 않는다.
- 나머지 태그 순서와 ID·제목·상태를 보존한다.
- 빈 태그·없는 ID·잘못된 CLI 인수는 실패한다. CLI 실패는 빈 stdout, 이유 있는 stderr, nonzero exit다.
- 공통 저장/경로 검사를 재사용하고 외부 경로 거부를 유지한다. assertion 삭제·완화 없이 README와 경계 테스트가 맞아야 한다.

## 순차 SDD 실행

현재 native Agent schema의 clauduct-inherit를 사용하고 model 인수는 생략한다. 생성 시점 직접 부모의 실제 모델·effort를 상속하며 Astra/sol 등으로 고정하지 않는다. schema에 없으면 임의 등록이나 다른 경로로 대체하지 말고 중단한다. 한 번에 자식 하나, 재위임은 금지한다. 각 자식에 명세·허용 파일·중단 조건을 전달한다.

1. 새 S22-spec-reviewer가 실제 diff, 관련 전체 소스, untracked test/untag.test.mjs를 읽고 명세 충족 여부를 보고한다. 읽기 전용이며 테스트·수정·커밋하지 않는다.
2. completed 결과로 명세 검토가 통과하면 새 S22-quality-reviewer가 오류 처리·멱등성·데이터 보존·경계 보호·공통 API·테스트 품질을 읽기 전용 검토한다.
3. 정상 완료한 검토에서 구체적인 결함이 발견될 때만 새 구현 자식이 위 다섯 파일 안에서 재현 검사·최소 수정·전체 회귀를 수행한다. 메인이 구현을 대신하지 않는다. 이후 새 명세/품질 검토를 거친다. 최대 보완 2회, 전체 자식 8개, 자식당 도구 30회·수정/재시험 3회다.
4. 두 검토가 통과하면 메인이 최종 diff와 새 파일을 읽고 같은 실행기로 전체 테스트를 실행한다. 기존 41개와 추가 사례가 모두 통과해야 한다.

## 비동기와 중단

async handle은 접수일 뿐이다. native 자동 완료 알림 경로이면 메인은 기다린다는 짧은 응답으로 턴을 끝내고 completed와 실제 결과가 도착해야 다음 단계로 간다. continue 입력, polling, Bash sleep, TaskOutput, SendMessage, 출력 파일 탐색, TaskStop, 중복 Agent로 기다림을 대체하지 않는다. 중복 완료 알림은 한 번만 처리한다. 결과 없는 실패는 재위임 근거가 아니다.

첫 API 오류·도구/환경 오류·guard 거부·인증/사용량 제한·예상 밖 기준선 실패·사용자 취소·결과 없는 자식 실패에서 후속 작업을 중단한다. 새 근거 없는 같은 목표 3회 실패도 중단한다. 보존된 변경을 되돌리거나 실패 상태를 성공으로 커밋하지 않는다. 오류 후 진단 요청·재시도·추가 자식·인증 조회를 하지 않는다.

## 기록과 완료

메인은 SDD_VERIFICATION.md에 과거 RED/GREEN의 출처와 현재 기준선, 검토 생성/완료 순서, 결함·보완, 최종 테스트 명령/exit/count, 미검증 항목을 기록한다. 실제 model/effort 및 부모 연결은 관측 증거가 있을 때만 확정하고 inherit 요청 자체를 라우팅 증명으로 삼지 않는다.

검증 통과 후 위 다섯 파일과 SDD_VERIFICATION.md의 의도된 변경만 명시적으로 stage해 run-02에 로컬 커밋한다. 최종 HEAD/status를 확인한다. 완료 문구는 SDD-PRESERVED-GREEN-REVIEW-PASS이며 Verified / Not verified / Blocked by와 커밋을 보고한다. 한 세션 무중단 SDD·native resume·제품 전체 완료를 뜻하지 않는다.

## 사용자의 종료 및 결과 수집

최종 응답 또는 중단 후 native의 정상 종료 기능으로 터미널에 돌아온다. 모델에게 continue나 진단 실행을 요청하지 않는다. ! node request-status.mjs 등 Bash 진단도 실행하지 않는다. 출력이 후속 모델 요청을 일으킬 수 있기 때문이다. 창을 강제 종료하거나 의도적인 취소 시험을 추가하지 않는다.

세션 ID, 최종 보고, Clauduct 종료 줄과 CLAUDUCT_REQUEST_STATUS JSON을 별도 검증 대화에 전달한다. cleanup과 모델 요청 성공은 별도로 판정한다. 종료 출력이 없으면 미확인으로 남기며 새 프로세스로 과거 진단을 복원하거나 시험을 자동 반복하지 않는다.
