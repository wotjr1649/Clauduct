# v0.6.1 — Workflow 동시 선택 안정화

2026-09-28, 동일 Workflow plan 자식의 동시 첫 요청이 검증된 선택을 재사용하도록 고쳤다.
첫 요청이 선택을 저장한 뒤 들어온 요청을 다음 plan 단계로 잘못 계산하던 경합이다.
기존 cache 조회를 공유하고 Workflow 잠금을 얻은 뒤 다시 확인한다(#191, [PR #193](https://github.com/wotjr1649/Clauduct/pull/193)).
session·parent·role·재개·native 중지 검증과 기존 실행·권한 정책을 유지한다.

유지보수 도구 `ptydrive`는 화면 일치와 별도의 완료 신호를 함께 기다릴 수 있다(#190,
[PR #192](https://github.com/wotjr1649/Clauduct/pull/192)). 선택적 화면 단계나 빈 패턴이 완료 신호의
timeout을 성공으로 바꾸지 않도록 검사한다. 이 도구는 배포 자산에 포함되지 않는다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 / commit | `v0.6.1` / `900b99e8521876ce99d16d5e38e22c41f8ee9b84` |
| 바이너리 SHA256 | `8a97045bc0e6cdd8d34d5f3568220034ac94ca297673756e37900905c58c660c` |
| 빌드 | Go 1.27.1, Windows amd64, CGO=0, trimpath, clean tag, 독립 캐시 재현 빌드 일치 |
| 측정한 환경 | Claude Code 2.1.283, Codex CLI 0.157.1, PowerShell 7.6.6 |
| 자산 | clauduct.exe · install.ps1 · uninstall.ps1 · SHA256SUMS |
| Release | [v0.6.1 정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.1) |

## 검증

| 범위 | 관측 결과 |
|---|---|
| 동시 선택 회귀 | 수정 전 동일 자식의 동시 요청 48개 중 2개가 거부됐다. 수정 후 결정적 재현과 동시 실행을 반복 통과하고, cache 재확인을 제거한 mutation은 실패했다 |
| 출하 소스 | 같은 공개 tree의 일반/race 각각 22패키지, gofmt·vet·build, PR 및 main CI 통과. 독립 backend 코드 리뷰의 마지막 결과는 finding 0이며 검토한 소스가 태그와 일치한다 |
| 최종 바이너리 | 필수 보고서 20개 PASS, 실제 backend 예약 214회. 예상 밖 API·도구·정리 실패 0. 취소·회복 표본의 의도적인 CANCELLED 1건은 예상 결과로 구분한다 |
| Workflow | 순차 plan-v1 및 병렬 raw script의 독립 파일 값·정확한 worker route·결과 전달, script/scriptPath 변경 후 native 재개, 완료 결과 회수 |
| Agent·Skill·설정 | UUID 재개, SDK fork 중첩, native 기본 중첩 후 명시적 SendMessage 재개, fork Skill 재개·중첩, 설정·UUID/fork 복원 |
| TUI | 중첩, 중간 handback 뒤 native 대기와 독립 표본, 압축·취소·복구, S 선택·재개, 권한 분류 |
| Background·세션 | 실제 자식 결과, native done과 연속된 완료 checkpoint, stop/attach·respawn·peer 메시지·정리, 설치본의 일반 세션 및 대기 중 중복 Agent 요청 |
| 설치·공개 업데이트 | PS7 새 설치·v0.6.0에서의 업데이트·되돌림, 다운로드 bytes와 API digest, 공개 updater·최신 no-op·old 파일 정리, 0.3.x updater의 안전 거부와 새 installer 이행 |
| 실제 설치 | 표준 사용자 설치 경로의 신원·SHA256·native 실행·doctor·업데이트 no-op, 사용자 설정·native 설정·PATH·대화·snapshot 보존 확인. 실제 설치본의 추가 backend 5회에서 입력·clear·자식 위임·결과 수신·정리 통과 |

준비 중 실패한 5개 표본의 79회 호출은 별도로 보존했고 최종 PASS에 합산하지 않았다.
검증 도구의 빈 선택 목록 처리와 종료 시점을 보완했다. 특히 native의 완료 표시와 marker만으로
후속 요청이 모두 끝났다고 판정하지 않는다. 루트 턴·자식의 실제 종료와 요청·대기열·메모리 해제를
연속된 checkpoint에서 확인한 뒤 다음 단계로 넘어간다. 검증 도구 변경에도 mutation 검사를 적용했다.

중간 handback 대표·독립 표본은 추가 개입 없이 native 최종 통지를 기다린다. 준비 표본에서 루트가
추가 SendMessage로 개입한 뒤 빈 응답과 선택 거부가 발생한 기록을 지우지 않았다. 그 경계를 이유로
빈 답을 무조건 허용하거나 신원 검증을 완화하지 않았다. 모든 임의의 개입·모델 응답이 성공한다는 뜻은 아니다.

Background 준비 표본에서는 marker가 있어도 native job이 working에 머문 경우를 FAIL로 남겼다.
최종 표본은 실제 자식 결과와 완료 사실을 함께 보고하고 native done 및 실제 종료를 확인했다.
native의 완료 판정을 Clauduct가 대신 쓰거나 강제로 바꾸지 않는다.

## 유지되는 범위

[v0.6.0의 지원 범위와 미실행 목록](RELEASE-v0.6.0.md)을 유지한다. 이번 20개 보고서는 패치 영향의
회귀 인수이며, v0.6.0에서 열거한 296개 식별자 전체를 새로 실행했다는 의미가 아니다.
Office의 복합 서식·GUI 렌더링·매크로·암호 문서와 미실행 native UI는 별도 검증이 필요하다.
SDK 기본 모드, native 권한 정책, 모델·effort 기본값과 Workflow의 명시적 재개 규칙은 변경하지 않았다.

기존 v0.6.0 태그·자산은 보존한다. 설치와 되돌리기는 [PACKAGING.md](PACKAGING.md),
설정 보존과 세션 재개는 [SETTINGS.md](SETTINGS.md)를 따른다.
