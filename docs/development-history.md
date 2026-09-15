# 개발 이력 기준

2026-09-09부터 현재 구현을 기준 커밋으로 보존한다. 이전에는 실제 저장소에 커밋이 없었으므로 이 기준 커밋은 과거 수정별 이력이나 전체 검증 완료를 뜻하지 않는다. 과거 분석·검증 기록은 audit-2026-09-08.md와 remaining-verification.md에 있다.

이후 개발은 기존 status와 diff 및 관련 커밋 확인 → 원인별 수정 → 관련 검사 → staged diff 검토 → 원인별 커밋 순서로 진행한다. 커밋 메시지에는 문제와 변경 목적을 적고 검증 결과 및 미검증 사항을 구분한다. 완료 보고에 커밋 ID와 변경 파일을 남긴다.

기준 커밋은 src, poc, bin, docs, verification, README.md, clauduct.cmd, .gitignore를 포함한다. 기존 clauduct-check.txt는 포함하지 않으며 삭제하지 않는다. 당시 함께 있던 %SystemDrive% 디렉터리는 이후 사용자 승인 아래 제거됐다. 원격 push는 수행하지 않는다.

커밋 생성으로 대상 파일이 tracked 상태가 되므로 review-diff는 이후 HEAD 대비 작업 중 변경을 수집한다. 변경이 없는 파일은 빈 diff가 정상이다. 과거 untracked 파일 전체 검토와 동일한 테스트로 해석하지 않는다.
