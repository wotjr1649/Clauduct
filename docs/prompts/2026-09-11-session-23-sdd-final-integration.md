# Session-23: 보존된 SDD 결과 최종 통합

## 목적과 범위

새 Clauduct 프로세스·새 세션에서 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02의 검토 완료된 untag 구현을 최종 검증·기록·로컬 커밋한다. 이전 세션을 resume하거나 이전 시험 프롬프트를 추가로 읽지 않는다. 이번은 새 SDD 개발이나 독립 리뷰 반복이 아니다. Agent/Workflow를 새로 생성하지 않고 메인이 남은 통합만 수행한다. 상위 필수 지침과 guard를 준수한다.

쓰기 범위는 run-02의 SDD_VERIFICATION.md 보완과 아래 명시된 여섯 파일의 로컬 Git stage/commit이다. 제품 코드·테스트·README 구현 변경, 전역 설정, 인증 조회, 설치, 외부 검색, push/PR/배포는 하지 않는다. 읽기 범위는 해당 저장소, 이 문서, D:/AIDEV/Clauduct/src/run-node-tests.ps1이다. 범위 안에서 해결할 수 없는 차이·결함은 보존하고 SAFE-STOP한다.

## 전달받은 증거와 미완료

세션 c4c222f8-9676-4d22-a7e8-72340225b868에서 clauduct-inherit 명세 자식의 completed/SPEC PASS 다음 품질 자식의 completed/QUALITY PASS가 확인됐다. 기준선 및 메인 검증에서 41 pass/0 fail을 관측했고 SDD_VERIFICATION.md를 작성했지만 최종 커밋 전에 API 오류로 중단됐다. 이 증거는 이전 세션의 검증자가 확인한 전달 증거다. 이번에 리뷰나 과거 RED/GREEN을 수행한 것으로 기록하지 않는다.

해당 gateway 전체는 요청 49건 중 성공 44건/실패 5건이다. 후반 요청 57은 UNSUPPORTED_EVENT other/identifier, 58은 REQUEST_STREAM_FALSE였다. 정상 종료·cleanup 통과는 오류 없는 SDD 완주를 뜻하지 않는다. 일부 자식의 sol/low definition-inherit 실행은 관측됐으나 모든 자식의 모델·직접 부모 연결까지 입증된 것은 아니다.

제품 커밋 339fc11은 자식 실행 한정 비스트리밍 fallback 차단, 최초 8개/최근 8개 실패 보존, requestOutcome 및 실행 정책 진단을 추가했다. 로컬 회귀 11개 파일은 통과했지만 실제 native fallback 차단과 최초 upstream 오류 해결은 미검증이다. 이 문서는 API 오류가 해결됐다고 가정하지 않는다. 세션 모델/effort는 실제 관측값만 인정한다.

## 한 번의 사전 확인

Git top-level이 run-02이고 독립 .git인지, HEAD가 298a5332e8f4f698002e236b020adf0d803f1562인지 확인한다. staged 변경은 없어야 한다. tracked 수정은 tasks.mjs, cli.mjs, README.md, test/boundaries.test.mjs이며 untracked는 test/untag.test.mjs와 SDD_VERIFICATION.md다. 다른 상태면 변경 없이 SAFE-STOP한다. 다음 해시는 최초 확인에만 적용한다.

| 파일 | SHA256 |
|---|---|
| tasks.mjs | DD8288A092C9E5CBC4CF2B018E4A8AA71C0C0124F6DE73BD9E8B7D8A5103A270 |
| cli.mjs | CCEB8503313150049C1B19248AB15840AD57B8FD4B3DC0BD96F370B672E55E3D |
| README.md | 2AC483F825D5268D4EF30B79B5257D848005A0A89BDF42685CC865BCE90BADE2 |
| test/boundaries.test.mjs | DC3B6ACE0EF03FC4731F0EEE09C1B9B0AAFECA5D7768204F3BD37D4EE27EFB1A |
| test/untag.test.mjs | CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3 |
| SDD_VERIFICATION.md | C9D666024CFDD27E4F7BEF21FB1543FB38954508C1B581DDC1F3D796B3C6C6DF |

## 최종 통합 순서

1. 실제 tracked diff, untracked 두 파일 전체, 필요한 관련 소스와 실행기를 읽는다. 이전 리뷰를 반복하는 작업을 만들지 않고 커밋 대상과 증거를 확인한다. 오류 처리·멱등성·경계 보호 assertion을 변경하지 않는다.
2. 아래 명령을 native Bash에서 한 번 실행한다. 실행기 자체는 60초 제한이며 도구 timeout은 그보다 여유 있게 설정한다. 기대값은 41 tests/41 pass/0 fail/exit 0이다. PATH·환경을 임의로 바꾸지 않는다.

```bash
pwsh -NoProfile -NonInteractive -File D:/AIDEV/Clauduct/src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct/verification/dev-sandbox/run-02
```

3. 통과하면 SDD_VERIFICATION.md에 이번 세션의 최종 통합 절을 추가한다. 이전 리뷰/RED/GREEN은 전달 증거, 이번 테스트는 직접 관측으로 구분한다. 이전 5건 API 실패와 미커밋 중단을 기록하고 한 세션 무중단 SDD·제품 전체 완료를 주장하지 않는다. 이번 명령/exit/count와 남은 native 검증 범위를 적는다. 아직 만들지 않은 커밋이나 종료 결과를 완료했다고 쓰지 않는다.
4. 최종 diff와 문서를 확인하고 git diff --check를 실행한다. 코드·테스트가 그대로이고 기록만 추가됐다면 동일 테스트를 또 실행할 필요는 없다. 위 여섯 파일만 명시적으로 stage한다. staged diff에 예상 밖 내용이 없으면 run-02에 한 번 로컬 커밋한다. git add . 또는 상위 Clauduct 저장소 커밋은 하지 않는다. hooks를 건너뛰거나 amend/reset/clean/restore로 상태를 정리하지 않는다.
5. 최종 HEAD와 status를 확인하고 SDD-PRESERVED-RESULT-INTEGRATED, 커밋, Verified/Not verified/Blocked by를 짧게 보고한다. 완료 기준은 보존된 결과의 통합·테스트·로컬 커밋이며 이번 세션이 SDD 자식을 새로 검증했다는 뜻은 아니다.

## 중단과 사용자 결과 수집

첫 API 오류·권한/guard 거부·도구/환경 오류·예상 밖 테스트 실패·사용자 취소에서 후속 실행을 중단한다. 동일 작업을 재요청하거나 wrapper·다른 shell로 우회하지 않는다. 문서 보완이나 stage까지 진행했다면 그대로 보존하고 실제 완료 범위만 보고한다. 결과 없는 커밋 요청을 무작정 재실행하지 않는다. 인증·usage 문제를 해결하려고 설정을 변경하지 않는다.

사용자는 최종 응답 또는 오류 후 native 정상 종료 기능으로 터미널에 돌아온다. 추가 continue, ! Bash 진단, 의도적 취소 시험은 하지 않는다. 세션 ID·최종 보고·Clauduct 종료 줄·CLAUDUCT_REQUEST_STATUS JSON을 별도 검증 대화에 전달한다. requestOutcome과 failureHistory(omitted 포함), clientExecutionPolicy, cleanup을 별도로 판정한다. 새 진단이 없거나 출력이 유실되면 미확인으로 남기며 시험을 자동 반복하지 않는다.
