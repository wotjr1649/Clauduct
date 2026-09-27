# v0.5.5 — PowerShell 7·phase 보존·분석 경고 정리

v0.5.5는 v0.5.4 감사에서 남은 항목을 보완한다. 묶음 C의 새 기능은 포함하지 않는다.
2026-09-28 [정식 latest Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.5)를 발행하고
실제 설치를 완료했다. 태그 commit은 `d17f7fe5b10971193df72894097f138a29a3a7d6`,
출하 바이너리 SHA-256은 `a0273e32787bdab212d9703a2202ffd93b24114c589e69efb3878984a43244fe`다.

- 설치·제거·Windows 검증 스크립트의 최소 버전을 PowerShell 7로 통일했다. 5.1 지원은 종료한다.
  5.1 호환용 hash·TLS 코드를 표준 PowerShell 7 명령으로 줄였으며 실행 정책을 변경하지 않는다.
- backend assistant 메시지의 `commentary`·`final_answer` phase를 native text block에 보존한다.
  스트리밍·대화 저장·UUID 재개·fork 후 원래 phase를 다시 backend에 보낸다. 없는 값을 추정하지 않고
  user/system/tool-result의 phase나 중간에 바뀐 phase는 거부한다.
- text block의 `phase`는 Anthropic 표준 밖의 Clauduct 확장이다. **이 필드가 포함된 새 대화는
  v0.5.5 이상에서 재개해야 한다.** v0.5.3/4는 명시적으로 거부하며 설정·snapshot·기존 이력은 보존한다.
  v0.5.5로 복귀하면 같은 UUID로 이어갈 수 있다. 기존 phase 없는 대화는 그대로 읽는다.
- 이미지/PDF의 공통 검증을 통합하고 쓰이지 않는 반환값·매개변수와 스타일 경고를 정리했다.
  미디어 허용목록·오류 분류·native PDF 오류 문구는 유지한다. 같은 uncapped 분석 설정에서
  dupl 2·staticcheck 62·unparam 16의 **80건을 0건**으로 줄였다. unused도 0건이다.
- native와 Clauduct가 담당하는 부분을 [아키텍처](ARCHITECTURE.md)에 구분하고, v0.5.4의
  중간 빌드에서 얻은 중복 차단 증거를 최종 바이너리의 증거와 구분했다.

초기 settings 생성·기존 파일 보존·S 선택·모델 매핑은 v0.5.4의 계약을 유지한다.
B의 native 분류기·추가 규칙·권한 설정도 유지한다. 지원 범위와 미검증 경계는
[호환성 문서](COMPATIBILITY.md), 설치 및 되돌리기는 [패키징 문서](PACKAGING.md)를 따른다.

로컬 전체 일반·race는 각각 22개 패키지, gofmt·vet·build·문서 인용과 uncapped 분석이 통과했다.
phase 해독·이력 재전송·snapshot 대조와 media 허용목록·PDF 문구·credential redaction 수정을
되돌린 검사는 실패했다. Sol/medium·Luna/medium·Terra/high의 실제 phase 왕복과 backend 계수도
통과했다. 실제 native ConPTY에서는 생성 3회·압축 1회·취소 1회와 이후 복구, 정상 종료 및
이력의 사실·순서를 확인했다. 이 TUI 검사는 외부 요청 없는 로컬 전송으로 native 동작을 검사한다.

최종 태그 바이너리에서 전체 Agent 안내를 유지한 중복 차단이 실제로 발동했다(`duplicateWait=1`).
native Agent는 한 번 실행됐고 결과 전달·최종 응답·정상 종료·메모리 해제가 확인됐다.
별도 정상 표본은 `duplicateWait=0`으로 같은 기준을 통과했다. 이미지/PDF·phase 포함 UUID 재개,
v0.5.3/4의 명시 거부와 파일 보존, v0.5.5 복귀도 같은 최종 bytes로 확인했다.

Go 1.27.1·CGO=0·trimpath의 깨끗한 태그를 독립 캐시 두 개에서 빌드해 바이트 일치를 확인했다.
PowerShell 7.6.6에서 신규 설치·v0.5.4 업데이트·되돌림, 완전한 초기 설정 생성과 사용자 편집
설정 보존을 확인했다. 공개 자산 4개의 API digest·다운로드 bytes·SHA256SUMS를 대조했으며,
공개 태그 설치·v0.5.4 updater·반복 무변경·v0.3.5 updater의 안전한 거부와 새 installer 전환도 통과했다.
실제 설치본의 신원·native 실행·doctor·backend 출하 세션과 기존 Clauduct/native settings·PATH 보존을 확인했다.

v0.5.5의 실제 backend 사용량은 개발·후보·최종 자산·설치 확인을 합쳐 **48 attempts**다.
기존 공유 원장의 누적은 **580 attempts**이며 최종 태그/설치본 검증 19회가 포함된다.
필요한 backend 검증의 상시 승인 아래 각 단계의 유한 상한과 검색 차단을 유지했다.
후보 검증기의 상대 경로·환경 누락과 첫 Windows CI의 cwd 오류는 실패 기록으로 남겼다.
완료된 유료 단계를 불필요하게 반복하지 않았고, 수정 뒤 필요한 검사를 다시 통과했다.
Windows PowerShell 5.1은 지원 종료이며 실행 PASS로 바꾸지 않는다. 측정한 native는 2.1.283이다.
