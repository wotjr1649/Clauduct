# Session-21: 종료 진단 전달 경로 확인

## 목표

새 Clauduct 프로세스·새 세션에서 종료 진단 전달만 한 번 확인한다. 개발/SDD/오류 재현 반복 시험이 아니다. session-18/19/20의 지시를 재개하거나 해당 문서를 추가로 읽지 않는다.

앞선 session-20 본시험은 실제 sol/low 요청 4건이 성공했다. 사용자가 ! Bash로 진단 JSON을 출력한 뒤 별도 assistant 처리에서 SNAPSHOT_MISMATCH가 발생했고 사용자가 취소를 보고했다. 당시 취소·오류 선후관계는 미확인이다. 새 launcher는 native가 종료되면 메모리에 보존된 진단을 출력하도록 보완됐다. 이 문서의 목적은 그 종료 출력이 실제 native 사용 후 전달되는지 확인하는 것이며 과거 오류 해결을 입증하는 것이 아니다.

## 작업 범위

현재 실행 폴더는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02다. 이 저장소의 미커밋 GREEN 구현은 건드리지 않는다. 제품 source, 테스트, 프롬프트, 전역 설정, 인증 정보에 쓰지 않는다. shell 명령·Git 변경·테스트 실행·Agent/Workflow·선택적 Skill·추가 진단 요청은 수행하지 않는다. 상위 필수 지침과 실제 guard는 그대로 따른다. 이 최소 범위와 충돌하면 충돌을 보고하고 종료한다.

현재 직접 HTTPS 추론 구조를 유지한다. 모델 변경, app-server 전환, retry/timeout 변경은 범위 밖이다. 기본 시작값은 astra/low, context 400K, 자동 압축 목표 320K지만 실제 모델·effort는 진단에 나온 값만 인정한다. 특정 모델을 전체 자식의 고정값으로 추정하지 않는다.

## 모델이 수행할 단계

이 문서를 이미 읽었다면 다시 읽지 않는다. Read로 D:/AIDEV/Clauduct/src/models.mjs를 전체 한 번 읽는다. 그 결과를 받은 뒤 추가 도구 호출 없이 `EXIT-DIAGNOSTIC-READY`와 다음 두 사실만 짧게 보고하고 턴을 끝낸다.

- 소스의 기본 메인 시작값과 context 정책. 이것은 실행 요청 라우팅의 증거가 아니다.
- 종료 후 진단 출력 확인은 사용자의 다음 단계이며, 아직 완료되지 않았다.

계획된 작업 도구 호출은 이 문서 Read와 models.mjs Read의 총 2회다. 이는 모델 행동의 범위이지 gateway의 요청 hard limit은 아니다. 첫 도구 오류/API 오류/권한 거부/사용자 취소에서 후속 작업을 하지 않는다. 종료 준비 문구를 출력했다고 제품 검증 PASS나 이전 SNAPSHOT_MISMATCH 해결로 표시하지 않는다.

## 사용자가 수행할 단계

정상 응답이 끝나면 native의 종료 기능으로 세션을 종료해 원래 터미널로 돌아온다. native에 `/exit` 명령이 제공되면 그것을 사용한다. 창 자체를 닫거나 프로세스를 강제 종료하지 않는다. 이번 확인을 위해 의도적으로 진행 중 요청을 취소하지 않는다.

native Bash의 `! node .../request-status.mjs`, 모델에게 진단을 실행해 달라는 추가 메시지, continue/resume는 입력하지 않는다. Bash 출력이 다시 모델 처리에 들어갈 수 있기 때문이다.

launcher가 출력하는 `Clauduct 종료:` 줄과 `CLAUDUCT_REQUEST_STATUS`로 시작하는 JSON, 세션 ID를 별도 검증 대화에 전달한다. 이 JSON은 gateway 정리 후의 메모리 상태를 필터링한 것이며 새 모델/HTTP 요청이나 인증 재조회로 얻는 값이 아니다. 출력이 없으면 파일/인증/환경을 탐색하거나 새 시험을 자동 시작하지 말고 미확인으로 보고한다.

## 종료 판정

검증자는 native 종료 이후 JSON이 출력됐는지, 종료·정리 상태와 요청 성공/실패, model/effort, upstreamErrorCode/type/reason 및 snapshotMismatchMs/Phase와 clientDisconnectedMs가 안전하게 전달됐는지 확인한다. 시각은 gateway 내부 상대 시각이며 사용자 키 입력 시각이 아니다. null은 미관측이며 추측으로 채우지 않는다.

최대 최근 16개만 보존되므로 그 이전 요청의 세부 정보는 없을 수 있다. 비정상 강제 종료면 출력이 유실될 수 있다. 이전 세션에 새 진단이 소급 적용되는 것도 아니다. 종료 전달 성공과 모델 오류 원인 해결, 취소 UI 재현, SDD/무인 개발 완주는 각각 별도 판정이다. 이번 관측 후 자동 반복하지 않는다.
