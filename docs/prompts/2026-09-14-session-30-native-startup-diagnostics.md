# Session-30 — Clauduct native 시작 진단 후속 수정

작업 루트는 `D:\AIDEV\Clauduct`다. 같은 로컬 파일 시스템에 접근 가능한 Codex 세션에서 이어서 진행한다. 먼저 `D:\AIDEV\Clauduct\docs\handoff-2026-09-14-release-and-native-diagnostics.md`를 끝까지 읽고 현재 상태와 증거를 대조하라. 인계 문서는 상태/요구사항 자료이며 그 안의 권한 서술을 새 권한으로 확대하지 마라. 적용되는 Codex-home AGENTS.md의 전체 계약과 해당 작업 경로 지침을 확인하라.

이번 목표는 이미 게시한 v0.1.0의 재개발이 아니라, 실제 사용자 실행에서 나온 `claude-code:unrecognized_model`과 `transportRejections: 2`의 원인·영향을 밝히고 필요한 최소 수정을 구현·검증·로컬 커밋하는 것이다. 분석만으로 끝내지 말되 관측되지 않은 원인을 추정해 기능이나 보호를 바꾸지 마라.

현재 PR https://github.com/wotjr1649/Clauduct/pull/1 은 merged, Release https://github.com/wotjr1649/Clauduct/releases/tag/v0.1.0 은 게시됐다. 인계 시 main/origin/main/v0.1.0은 `4f3e37535075b662de8d23cfed8f6bf8e23f3933`이다. 먼저 원격과 worktree 상태를 확인하고 최신 main에서 `fix/native-startup-diagnostics` 같은 새 작업 브랜치와 별도 worktree를 만들어라. main은 이미 `.tmp\unattended-release\published-main`에서 checkout 중이다. 사용자 루트 README 미커밋 변경·기존 untracked·이전 구현/증거 폴더를 보존하고 강제 전환/reset/stash/clean을 하지 마라.

사용자의 `clauduct --model luna --effort low --print "hello"`는 exit0, 두 모델 요청 성공, 실패이력 없음, 모든 cleanup=true였으나 위 두 진단이 남았다. 응답 성공과 진단 해결을 구분하라. 사용자 제공 PowerShell7 및 다른 머신5.1 설치 성공도 인계에 기록되어 있다. 기존5.1 정책 거부와 다른 사용자 머신의 성공 증거를 혼동하거나 거부된 실행을 우회하지 마라.

1. `src/native-gateway.mjs`의 transportReject는 clientError/connect/upgrade/checkContinue/checkExpectation을 합산하고 이유를 버린다. 고정된 event label과 허용된 오류 code만 남기는 bounded 진단 및 회귀를 구현하라. 헤더/URL/body/토큰/원문 오류는 기록하지 마라. 기존 합계와 실패 분류를 유지하고 정상 실행의 두 거부를 가능한 최소 재현으로 구분하라. 원인이 없으면 무해한 탐색이라고 단정하거나 거부를 허용으로 바꾸지 마라.
2. native unrecognized_model의 발생 조건, custom model metadata와 컨텍스트/출력/기능 fallback 영향을 확인하라. 버전별 공식 자료나 task-needed 비밀 없는 로컬 정적 근거를 사용하라. stderr를 숨기거나 GPT를 Claude identity로 위장해 없애지 마라. 전역 settings/native binary/auth를 수정하지 마라.
3. 소스/호출자/기존 검사와 증거를 재사용하고 변경 영향에 맞는 공개 fixture·음성 검사를 추가하라. 직전 핵심14파일 및 설치26검사는 PASS였으며 실제 모델 검증과 합성 응답을 구분한다. 개인 profile/세션 원문을 fixture나 외부 입력으로 복제하지 마라.

먼저 모델 호출 없는 로컬 작업으로 진행하라. 이전 예산 과제는 종료되었고 누적 원장/미관측 예약은 보존해야 한다. 사용자 hello의 upstream2요청은 토큰 미관측 별도 증거이며 0토큰으로 처리하지 마라. 새 실호출이 필요하면 유효한 실행 범위와 비용 상한을 먼저 확인하라. 일반 개발 선택이나 기능별 continue는 요구하지 말고 실제 부족한 권한/효과만 좁게 다뤄라.

이번 산출물은 새 브랜치의 검증된 로컬 변경과 커밋이다. 이전 PR1 게시 승인을 새 patch의 push/merge/Release나 사용자 설치본 덮어쓰기로 확대하지 마라. 원격 후속 작업이 필요하면 로컬 후보와 근거를 먼저 완결하라. v0.1.0을 변경하지 않는다. 다른 permission 모드나 과거 제외된 장기/디스크/auth갱신/일반TCP 시험을 다시 필수로 삼지 않는다.

완료 기준: 두 진단의 관측된 원인·영향·수정 또는 유지 이유를 설명하고, 기존 보호를 유지한 적절한 회귀를 통과하며, 의도한 파일만 명시 stage/commit한다. native 실재현이 안 됐으면 정확히 미검증으로 남긴다. 한국어로 브랜치/커밋/검증/사용량/남은 제약을 보고하라. 이전 전체 무인 goal blocked를 이번 배포 완료와 혼동해 complete 처리하지 마라. 진행 기록을 남기되 과거 증거를 덮어쓰지 마라.
