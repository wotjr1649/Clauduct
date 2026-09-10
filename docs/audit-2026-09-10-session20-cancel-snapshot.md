# session-20 판정과 후속 보완 범위

## 변경 전 판정

5e6f5952-dd05-448b-b0b6-5e5dc00a8b88의 본시험은 2026-09-10T09:58:22Z에 READONLY-ROUNDTRIP-COMPLETED로 종료했다. 제공된 gateway 진단은 실제 gpt-5.6-sol/low 요청 4건 성공, 실패 0건, 모두 HTTP 200·단일 시도·completed였다. context 환경은 window=400000, autoCompactWindow=400000, compactPercent=84.21052631578947이었다. 과거 오류의 원인은 미확인이다.

2026-09-10T10:01:26Z에 사용자가 native Bash 모드로 request-status.mjs를 실행했다. 진단 JSON 출력 후 10:01:32Z에 후속 assistant의 SNAPSHOT_MISMATCH가 기록됐다. 사용자는 이 후속 처리를 의도적으로 취소했다고 보고했다. 취소의 gateway 도달 시각, mismatch와의 선후관계, mismatch 항목은 당시 진단에 없어 확정하지 않는다. 성공 4건의 JSON은 이 오류보다 앞서 수집됐다.

진단 Bash 실행을 후속 모델 처리와 분리할 수 있는 것처럼 안내한 것은 부정확했다. 이번 기록에서 실제 후속 assistant 처리가 발생했으며 새로운 개발 도구 실행은 확인되지 않았다. 단순 ! 명령을 모델 요청 없는 진단 수집 보장으로 사용하지 않는다.

## 승인된 후속 범위

먼저 이 판정을 커밋하고, 그다음 기존 sanitizer를 재사용하는 모델 요청 없는 진단 수집 경로 및 취소/mismatch의 처리 순서를 구분하는 로컬 회귀 검사를 구현한다. 정상 취소를 오류로 포장하거나 실제 mismatch를 취소로 숨기지 않는다. snapshot 검증 자체를 완화하지 않는다. 실제 authenticated native 시험이나 app-server 전환은 수행하지 않는다. run-02의 미커밋 구현과 기존 프롬프트는 보존한다.

이 문서의 초기 커밋은 구현 완료나 실제 취소 오류 해결 증거가 아니다. 후속 검증 결과를 별도로 추가한다.
