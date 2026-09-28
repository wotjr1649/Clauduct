# v0.6.0 — native Workflow 재개와 Bundle C

2026-09-28 최종 바이너리의 필수 인수·정식 Release·실제 설치 확인을 완료했다.

- 검증된 원 Workflow run에 source와 `resumeFromRunId`를 함께 전달하는 명시적 native 재개를
  지원한다. 수정한 script를 허용하며 완료된 앞 단계의 cache를 재사용한다. 실패·중단·prompt 변경
  지점 이후 효과는 반복될 수 있다. 기존 결과 회수와 plan-v1의 미실행 단계 재개는 유지한다.
- `EnterWorktree`·`ExitWorktree` 뒤 snapshot과 context journal이 native의 새 transcript 경로를
  따르도록 고쳤다(#185). 같은 UUID·경로·소유권 검증과 native 권한 규칙을 유지한다.
- native가 중간 `SubagentHandback`을 받은 뒤 손자가 남아 있으면 기존 native 대기를 이어 간다(#187).
  보고 전달을 전체 완료로 간주하지 않는다. native의 1회 보고와 후속 `SendMessage` 규칙을 유지한다.
- DOCX·XLSX·PPTX의 읽기 → 한 곳 편집 → 독립 파일 검사를 기존 native 도구 경로에서 검증했다.
  편집 라이브러리는 검증용 로컬 환경에만 준비했으며 제품 의존성을 추가하지 않았다.
- 파일 대상 native `/code-review`의 tracked·untracked 표본을 검증하고 V1 review-diff 헬퍼를
  문서상 은퇴했다. HTTP 200 응답을 받은 요청의 실제 body byte 수를 진단에 추가했다.
  본문·header·자격 증명은 기록하지 않으며 계수·검색·HTTP 실패의 bytes는 미측정이다.

SDK의 기본 모드는 바꾸지 않았다. 기본 중첩에서는 실제 손자 결과를 확인한 뒤 중간 Agent를
`SendMessage`로 재개하는 절차를 사용한다. 별도 자동 retry나 TUI 자동 종료는 추가하지 않았다.
설정·권한 분류·phase의 기존 계약은 [SETTINGS.md](SETTINGS.md)와
[COMPATIBILITY.md](COMPATIBILITY.md)를 따른다.

## 출하 신원

- [제품 PR #186](https://github.com/wotjr1649/Clauduct/pull/186),
  [handback 수정 PR #188](https://github.com/wotjr1649/Clauduct/pull/188).
- 태그 `v0.6.0`, commit `28d14b861ba7f456a86d0975426dbed851650126`.
- 실행 파일 SHA-256: `7936e3db4abefb63c50e52a668064a7aa296b9fc9c3a58b3c82b70ff2ead28de`.
- [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.0)는 latest이며
  `clauduct.exe`, `install.ps1`, `uninstall.ps1`, `SHA256SUMS` 네 자산을 제공한다.
- Windows amd64, Go 1.27.1, CGO=0, `-trimpath`. 깨끗한 태그와 독립 build cache에서 두 번 만든 bytes가 같다.
- Claude Code 2.1.283, Codex CLI 0.157.1, PowerShell 7.6.6에서 확인했다.

## 실행한 검사

| 범위 | 관측 결과 |
|---|---|
| 로컬 회귀 | 전체 일반·race 각각 22개 패키지 PASS, gofmt·vet·build PASS |
| 정적 분석·리뷰 | unused·unparam·dupl·staticcheck(all)의 제품/테스트 포함 분석 0 issues. 작성자 STATIC_REVIEW 수행; 독립 검토자가 검토한 것으로 표시하지 않음 |
| 회귀 검출력 | Workflow 5개·요청 bytes 2개·Worktree 2개·handback 2개, 서로 다른 수정 제거 검사 11개에서 해당 회귀 검출 |
| 통합 | PR 및 제품 병합 commit CI PASS, 태그 소스와 검토한 제품 tree 일치 |
| 최종 바이너리 | 같은 SHA-256에 연결한 필수 보고서 40개 PASS. 실제 backend 검사와 무과금 거부·경계 검사를 구분 |
| Workflow | 실패·중단·수정 script·scriptPath·결과 회수·계획 재개와 별도 독립 표본 PASS; 실제 로컬 효과와 native journal 대조 |
| Office | 세 형식의 원 표본·독립 표본 PASS. ZIP CRC·XML 관계·변경 part·재개방 결과를 모델 답변과 별도로 검사 |
| `/code-review` | tracked·untracked 파일의 원 표본·독립 표본 PASS. 실제 검토 내용·반례·파일 무변경·요청 bytes·사용량·시간 확인 |
| 모델·도구 | 4개 모델×5 effort, 파일 도구·NotebookEdit·구조화 응답·MCP·공개 WebSearch/WebFetch PASS |
| Agent·Skill | SDK/TUI 위임·중첩·명시적 재개·forked Skill·중간 handback 대기 PASS. 중간 보고 이후 실제 손자 결과와 최종 루트 결과 확인 |
| 세션·background | 설정·plugin pair·S·UUID/fork/clear, background stop/attach·worker respawn·peer 결과 수신·정리 PASS |
| native 경계 | Worktree 생성/기존 tree 보존, 권한 허용/거부, 계획·Cron 생성/조회/삭제, 취소·복구 PASS |
| media·phase | 이미지/PDF·압축 후 후속 입력, phase UUID 재개·v0.5.3/4 거부·현재 버전 복귀 및 합성 settings/snapshot/이력 보존 PASS |
| 정상·반복 위임 | 각각 실제 자식 1개. 정상 표본 중복 차단 0, 반복 요청 표본 중복 차단 1; 완료 수신·메모리 해제·정상 종료 |
| 배포·설치 | API digest·공개 다운로드 bytes 일치. PS7 새 설치·v0.5.6 업데이트/되돌림·공개 태그 설치·updater·no-op·잔여 파일 정리 PASS |
| 이전 updater | v0.3.5가 없는 사본 자산을 거부하고 기존 설치 보존, 새 installer로 v0.6.0 복귀 PASS |
| 실제 설치본 | 버전·commit·SHA·native 실행·doctor 확인. 실제 backend 5회로 응답·clear·자식 1개·결과 수신, API/도구 실패 0 및 정상 종료·메모리 해제 |
| 사용자 상태 | 실제 설치본의 updater no-op/native version/doctor 전후 설정·native 설정·대화 파일과 PATH 전체 해시 동일. 사용자 snapshot 디렉터리의 기존 부재 상태 유지 |

정상 성공 표본은 API·도구 실패 0과 정리를 요구했다. 의도한 도구 오류·권한 거부·취소는 별도
시나리오의 기대 결과로 기록했다. 최종 중첩 표본의 조정 Agent는 `gpt-5.6-terra/medium`,
leaf는 `gpt-6-luna/low` 또는 `medium`이며 루트는 `gpt-6-sol/medium`이었다.
이 조합의 관측 결과를 모든 모델·프롬프트의 의미적 정답 보장으로 확대하지 않는다.

## 비용과 남은 검증 범위

하나의 누적 원장을 사용했다. 이번 작업은 **876→2077, 실패 포함 1201회 예약**이다.

| 단계 | 예약 증가 |
|---|---:|
| Bundle C 개발 | 328 |
| 폐기한 첫 출시 후보 | 257 |
| 중간 handback 결함 보완 | 226 |
| 최종 출시 bytes의 필수 인수·실패·회복 확인 | 385 |
| 실제 설치본 backend | 5 |

단계별 유한 상한·시간 제한·실패 조사 조건을 유지했다. 종료 시 원장 상한을 실제 누적 2077로
닫았고 검색 허용은 false다. 예약 수는 금액이나 성공 응답 수가 아니다.

식별자는 **296개**를 열거했고 직접 사용과 해당 기능군의 필수 근거가 있는 **63개만 PASS**,
나머지 **233개는 NOT_RUN**으로 기록했다. 기능군 대표 검사와 개별 native UI·옵션·skill 실행은
다르다. 외부 계정/서비스 연동 전체, Office GUI·매크로·암호 문서·모든 복합 서식 보존은 이번 PASS에
포함하지 않는다. 기존 지원 범위를 미지원으로 바꾸지 않았으며 개별 실측은 후속 작업이다.

초기 준비 입력·종료 동기화 오류, 모델의 1회 handback 규칙 위반, 실제 연결/stream 실패는 FAIL로
보존했다. 첫 설치 보존 검사도 별도 `claude-mem` worker의 동시 대화 기록 변경을 감지해 FAIL했다.
다른 프로세스나 기록을 건드리지 않고, 동시 쓰기 예외 없이 전체 해시 검사를 다시 통과했다.
재검사에는 추가 backend 호출이 없었다. 기존 실패를 소급 PASS로 표시하지 않는다.

Workflow의 외부 효과는 native 재개 규칙에 따라 반복될 수 있으며 exactly-once를 보장하지 않는다.
확인한 범위의 정상 동작·오류 처리·정리를 검증한 결과이며 모든 입력의 무결함을 보장하지 않는다.
