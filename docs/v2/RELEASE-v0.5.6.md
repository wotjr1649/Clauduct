# v0.5.6 — native 순서 복원과 잔여 품질 보완

v0.5.5 재감사에서 발견한 잔여 항목을 보완하고 2026-09-28 출하·설치 인수를 완료했다.
묶음 C의 새 기능은 포함하지 않는다.

- native 이벤트 모듈이 reload·worker respawn으로 다시 등록되면 기존 게시 순번을 이어 쓴다.
  시계 역행·동일 시각뿐 아니라 시계가 증가했어도 기존 누적 순번보다 작았던 경우의 요청 거부를 고친다.
  같은 agent의 게시를 직렬화하고 최신 순번이 충돌하면 거부한다. 부분 쓰기·취소·재실행 방지 계약은 유지한다.
- 제품 전용 분석에서 확인한 미사용 선언 6개, 추가 검토에서 확인한 테스트 전용 조회 메서드 4개,
  그 정리 후 드러난 불필요한 반환값을 제거했다. 공용 테스트 전송 도구도 제품 패키지에서 분리했다.
  테스트는 native turn 검증·등록 조회·요청 취소의 실제 제품 경로를 사용한다.
- 보관 PowerShell 스크립트에도 7 요구사항을 명시하고, 이전 검증 실행기의 5.1 PATH 잔재를 7로 바꿨다.
  옛 스크립트의 용도와 당시 관측 결과는 보존한다.
- 압축 직후 backend가 media 입력의 길이를 거부했을 때 진단 카운터에서 빠지던 경우를 집계한다.
  기존 오류와 반복 압축 중단을 유지한다. 이 집계 수정은 모든 media 크기를 사전에 정확히 안다는 뜻이 아니다.
- native auto 권한 분류 전용 경로를 `gpt-6-luna/high`에서 `gpt-5.6-terra/high`로 바꿨다.
  승인된 PR/Release를 분류 transcript 밖의 승인과 혼동해 차단하던 경우를 모델 선택으로 보완했다.
  native 지침·기본 규칙·추가 규칙 두 개·응답 계약은 그대로이며 일반 모델 매핑에도 영향이 없다.
  기존 70개 표본은 위험 허용 0/30·정상 허용 30/30·형식 오류 0,
  후보 동결 후 새 독립 표본은 위험 차단 8/8·정상 허용 8/8을 기록했다.

기존 settings 생성·기존 파일 보존·S 선택·UUID/fork/clear 정책과 v0.5.5의 phase 호환 범위를 유지한다.
media의 추가 사전 계수를 넣지 않는 기존 정책을 유지한다. 이미지/PDF의 길이를 사전에 모두
예측하지 못할 수 있으며, 압축 직후에도 실제 backend가 길이를 거부하면 자동 재시도 없이 중단한다.

## 출하 신원

- [제품 PR #183](https://github.com/wotjr1649/Clauduct/pull/183), 태그 `v0.5.6`, commit `57f22f6e4bc5d25fd24d82f24139355db9a3c10f`.
- [정식 latest Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.6)의 자산은 `clauduct.exe`, `install.ps1`, `uninstall.ps1`, `SHA256SUMS` 넷이다.
- 실행 파일 SHA-256: `e4eb573c04dad0c79b8880fd019e64acc6e3a9d293461bfcc910ab2fac8bd4a4`.
- Windows, Go 1.27.1, CGO=0, trimpath. 깨끗한 태그와 별도 build cache로 두 번 빌드한 bytes가 같다.
- Claude Code 2.1.283, Codex CLI 0.157.1, PowerShell 7.6.6에서 확인했다.

## 완료한 검사

| 범위 | 관측 결과 |
|---|---|
| 전체 일반·race | 모두 PASS. 취소·replay·설정·UUID/fork/clear·Workflow 회귀 포함 |
| gofmt·vet·build | 일반·policy_evidence·runtime_evidence 검사 PASS, PR·병합 commit CI PASS |
| 동일 정적 분석 | unused·unparam·dupl·staticcheck(all), 테스트 포함/제외 각각 0 issues. 제품 AST의 4 statements 이상 동일 body 0. 표준 interface의 GoString/MarshalJSON은 유지 |
| 순번 회귀 검출력 | 이전 publisher·직렬화 제거·동률 guard 제거 각각 의도한 검사 FAIL, 현재 제품 PASS |
| native TUI | 실제 reload·clear, 과금 없는 압축·취소·복구 PASS. 실제 분류기의 합성 쓰기 허용·삭제 거부 PASS |
| B 모델 변경 | 기존 70개·동결 후 독립 16개·기존 오차단 4개 재검사 PASS. 출시 실행 파일의 실제 TUI에서도 Terra/high 분류 2회와 승인된 파일 쓰기 확인 |
| 실제 background | worker 재시작 전후 자식 보고서 2개 수신, 실패 0, native 최종 done·정리 PASS |
| 설정·플러그인·Workflow | 완전한 초기 문서, native 설정 출처, plugin pair, UUID snapshot 보존, 새 기본값, Workflow pair, alias 기본 effort 실제 backend PASS |
| 압축 후 media | 기존 집계 누락 재현→수정 후 1건 집계. 실제 초기 요청→압축→이미지 후속 요청 PASS, 추가 사전 계수 0. 실제 서비스 한도 초과를 유발한 검사는 아님 |
| 출시 bytes의 Agent | 독립 정상 표본은 중복 차단 0, 반복 요청 표본은 중복 차단 1. 둘 다 실제 child 1개·완료 수신·최종 응답·메모리 해제·정상 종료 |
| 출시 bytes의 phase·파일 | phase UUID 재개·이미지/PDF, v0.5.3/4의 phase 거부 및 설정/snapshot/이력 보존, v0.5.6 복귀 PASS |
| PowerShell 7·설치 | 30개 ps1 AST·7 요구사항·legacy 실행 참조 검사 PASS. 새 설치·설정 보존·업데이트·v0.5.5 되돌림·공개 태그 설치·updater·no-op PASS |
| 이전 updater | v0.3.5의 자산 누락 거부·원본 보존 후 새 installer로 복귀 PASS |
| 공개·실제 설치 | API digest·다운로드 bytes 일치. 실제 설치본 버전/hash/native/doctor, 별도 Agent 세션 PASS. 사용자 settings·native settings·PATH hash 보존 |

이번 작업의 실제 backend 예약은 실패·재시도를 포함해 **296회**, 기존 공유 원장은 **580→876회**다.
마지막 단계 상한 892 안에서 종료했으며 검색은 허용하지 않았다. 설정·background 검사는 실제 제품
코드와 native/backend 조합, 출시 실행 파일 검사는 위에서 따로 표시한 범위다.

초기 검증 도구의 잘못된 native Git 상태 가정과 변경된 분류 route 목록 누락도 수정하고 재검증했다.
실패 기록을 성공으로 덮지 않았다. 보관 스크립트는 PS7 실행 경계를 검사했으며 역사 버전의 설치를
전부 다시 실행한 것은 아니다. 과거 NOT_RUN을 소급 PASS로 표시하지 않는다.

#178·#179·#180·#181·#50의 관측 결함과 합의한 지원 범위를 처리했다. 예측 불가능한 모든 media
용량이나 모든 가능한 프롬프트까지 무결함을 보장하지 않는다. 묶음 C의 새 기능은 별도 이슈로 남는다.
