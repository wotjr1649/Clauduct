# v0.6.15 — native 2.1.330 호환성 수정과 PDF 전송 단순화

`<RELEASE_DATE>` 출시(태그 `v0.6.15` → `<TAG_COMMIT>`, [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.15)). 출하 기록 초안이며 태그·자산 신원과 검사 결과는 발행 기록으로 확정한다.

범위는 native 2.1.330에서 발견한 호환성 결함 수정(역할 선택, Workflow 권한, PDF 페이지 Read)과 [#331](https://github.com/wotjr1649/Clauduct/issues/331)의 2.1.330 재고정, PDF 전송 단순화다. v0.6.14는 native 2.1.330에서 아래 결함이 있었다.

## 바뀐 것

- **native 2.1.330에서 subagent 역할 선택이 거부되던 결함을 고쳤다.** 2.1.330은 자식 요청의 metadata에 모델 ID를 채운다. 이 값이 native가 실제로 고른 모델과 정확히 같으면 허용한다. 환경변수·settings로 모델을 지정한 general-purpose·Explore·Plan, 역할 표에 없는 내장 역할, ZIP 플러그인 역할이 `AGENT_SELECTION_UNVERIFIED`로 막히던 문제다. 다른 모델은 계속 거부한다.
- **Workflow 파일 소스는 파일별 Read 금지가 확인된 native에서만 허용한다.** 2.1.330은 Read를 금지한 Workflow 소스 파일도 읽었다. 이 빌드와 확인되지 않은 native에서는 `scriptPath`·이름 지정 Workflow를 Read 전에 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 거부한다. inline Workflow는 그대로 쓸 수 있다.
- **PDF는 원본만 보낸다.** 첨부와 Read 결과의 PDF를 backend가 직접 읽게 원본 파일 하나로 전달한다. 이전에는 모든 페이지를 PNG로 렌더링해 함께 보냈다. 실제 문서 비교에서 정답률은 같고 입력 토큰은 약 절반이었다. 내장 PDF 렌더러, Windows용 `pdftoppm` 별칭과 그 별칭을 위한 PATH 주입을 제거했다. [#352](https://github.com/wotjr1649/Clauduct/issues/352).
- **native 2.1.330에서 background·관리 명령 인자 판정을 native와 맞춘다.** 실행 파일의 SHA-256이 내장 표의 2.1.330 값과 일치하면, 옵션 값 자리의 `--bg`(예: `--name --bg`)와 옵션 값으로 소비되지 않은 첫 `--` 뒤의 `--bg`는 foreground로 처리한다. 허용된 옵션 뒤의 관리 명령(`clauduct --verbose rm <id>` 등)과 선두 `kill`은 native로 바로 보낸다. 2.1.330에서 새로 생긴 값 옵션 `--initialize-fields`·`--metadata`와 불리언 `--hold-repository-while-parked`의 경계도 반영했다. 다른 실행 파일에서는 기존 보수 경로를 쓴다. [#331](https://github.com/wotjr1649/Clauduct/issues/331), [ARCHITECTURE §4](ARCHITECTURE.md#4-cli-계약).
- **`--metadata --bg --help`처럼 값 자리에 있는 `--bg`는 도움말 단축 경로를 막지 않는다** (2.1.330 고정 빌드에서만).

## 출하 검사

이번 출하부터 태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 판정을 기록한다. 아래 항목은 결과를 대조할 목록이다.

- [ ] `vet`, 전체 오프라인 Go 테스트.
- [ ] 태그 fixture `fixtures`·`fixtures-bg`, `structural`, native smoke 15행.
- [ ] 변경 동작의 실제 backend 표본 `s331`(background·관리 명령), `roles`(subagent 역할 선택), `workflow`(파일 소스 거부와 inline 실행), `PDF`(짧은 PDF 페이지 Read).
- [ ] `c1` 무과금 도구 이름·core 재측정. 직전 유효 T1/T2와 native 이미지가 달라졌을 때 실행한다.
- [ ] 동시성 코드가 바뀐 패키지의 race. 실행하지 않은 단계는 `NOT_RUN`과 이유를 기록한다.

### 출하 빌드·발행 검사

[PACKAGING §4.1](PACKAGING.md#41-릴리스-만들기)의 절차에 따라 출하 자산과 검사 기록을 대조한다.

- [ ] `build-release`. 두 독립 빌드의 바이트와 probe digest를 대조한다.
- [ ] 발행 전 `ship-install`. 새 설치, v0.6.14에서 업데이트, 되돌리기와 사용자 PATH 불변을 확인한다.
- [ ] 발행 전 `probe limit --send` 3회. backend의 `max_output_tokens` 상한 적용을 확인한다.
- [ ] 발행 뒤 `verify-release`. 내려받은 자산과 API digest·`SHA256SUMS`를 대조한다.
- [ ] 발행 뒤 `ship-update`·`ship-legacy`. 이전 릴리스의 `--update`와 `already current`, v0.3.5에서의 이전 경로를 확인한다.
- [ ] 발행 기록 병합 뒤 `post-publish`. 최신 Release·태그·자산을 현행 인덱스, 이 문서의 출하 신원, 호환성 원장과 대조한다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.15` → `<TAG_COMMIT>` |
| `clauduct.exe` | `<ASSET_SHA_EXE>` |
| `clauduct-probe.exe` | `<ASSET_SHA_PROBE>` |
| `install.ps1` | `<ASSET_SHA_INSTALL>` |
| `uninstall.ps1` | `<ASSET_SHA_UNINSTALL>` |
| `SHA256SUMS` | `<SHA256SUMS>` |

## 알려진 제한

- **10쪽이 넘는 PDF를 페이지 지정으로 읽으려면 Poppler(`pdftoppm`)가 필요하다.** native Claude Code와 같은 동작이다. 설치되어 있지 않으면 native가 해당 Read를 오류로 돌려준다. 페이지를 지정하지 않는 Read와 첨부는 영향이 없다.
- **native가 설정·로그인 상태에 따라 background 요청을 root 경로로 넘기는 분기는 실제로 발동시켜 확인하지 못했다.** Clauduct는 이 경우에도 background 요청으로 판정하고, 실제 처리는 native에 맡긴다. [#331](https://github.com/wotjr1649/Clauduct/issues/331).
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [v0.6.14 알려진 제한](RELEASE-v0.6.14.md#알려진-제한).
