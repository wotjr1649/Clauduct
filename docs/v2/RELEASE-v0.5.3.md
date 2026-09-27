# v0.5.3 — Clauduct 설정과 세션 선택

2026-09-27 정식 latest Release와 실제 설치 확인 완료. 태그 `v0.5.3`은
`788a60ed2513785f036769441f0105b8d37dbb55`이며 [구현 PR #159](https://github.com/wotjr1649/Clauduct/pull/159)에서 병합했다.

Clauduct의 모델 설정을 수동 `.clauduct/settings.json`에서 관리한다. 새 세션은 기본적으로 Sol/xhigh로 시작한다.
`startup`, GPT별 `modelDefaults`, Claude alias의 `modelMapping`, 개별 `agents`의 model·effort를 분리했다.
설정 형식과 우선순위는 [SETTINGS.md](SETTINGS.md)에 있다.

설치·업데이트와 일반 실행은 설정 파일이 없을 때만 `{"version":1}`로 생성한다. 기존 파일은
유효성이나 버전에 관계없이 덮어쓰지 않는다. 구버전 updater에서 올라오면 새 버전의 첫 일반 실행이나
다시 실행한 `--update`가 생성을 보완한다. 정보 조회 명령은 설정을 만들지 않는다.

사용하지 않는 SSE 완료 setter·thought 계수·fixture 판별 helper를 제거하고, launcher·doctor·hook의
환경변수 변환은 기존 platform 함수로 모았다. 새 의존성은 추가하지 않았다.

native의 `S` 선택은 세션에 보존한다. 질문을 보내지 않고 종료해도 마지막 모델·effort를 기록하며,
UUID를 지정한 재개에서 당시 설정과 선택을 복원한다. 새 agent도 그 세션의 설정을 따른다.
fork는 원본 설정을 복제하고 clear는 현재 실행의 설정을 유지한다.

snapshot이 없는 이전 세션은 현재 시작값으로 열며, 사용자는 S로 바꿔 계속할 수 있다.
snapshot이 있는 세션의 재개 목록·continue·실행 중 resume에서는 정확한 UUID 재실행 명령을 안내한다.
자동 재실행 후보는 입력 손실이 재현되어 채택하지 않았다.

여러 Claude alias를 같은 GPT에 매핑해도 명시적인 GPT 모델 선택을 유지한다. plugin agent의
prompt·tools·권한은 native가 적용하며, Clauduct의 agent pair와 명시적 호출 선택을 구분한다.
native 전역 개인화, B의 추가 차단 규칙 두 개와 `$defaults`, Luna/high classifier 정책은 유지한다.

검증:

- Go 1.27.1, Windows, native 2.1.283에서 gofmt·vet·build·전체 일반 검사·전체 race 검사 통과.
- 실제 native/Fixture에서 S·즉시 Ctrl+C·UUID 재개·fork·clear·비영구 재개와 이전 세션의 네 CLI 경로 확인.
- 별도 표본에서 임시 관찰 plugin·status line 없이 마지막 effort 복원 확인.
- 실제 backend 20 attempts로 plugin pair, 설정 편집 후 snapshot 재개, 새 실행의 변경값, Workflow,
  Sol/xhigh 기본 시작, Sonnet→Luna/low 매핑 확인. 모든 사례에서 실패·거부 0.
- 설정 생성·정리 변경 뒤 추가 1 attempt로 파일 없는 첫 실행의 생성과 Sol/xhigh 응답 확인. 누적 21회.

출하 검사:

| 검사 | 결과 |
|---|---|
| Go 1.27.1, CGO=0, trimpath, 깨끗한 태그 | 독립 캐시 두 개에서 동일 바이트, 의존성 검증 통과, `+dirty` 없음 |
| 순수 출하 바이너리 실제 backend | 5 attempts, 입력·clear·background child·최종 응답 PASS. 자식 선택 검증, 실패 0, 정상 종료·메모리 회수 확인 |
| 공개 Release 자산 | `clauduct.exe`, `install.ps1`, `uninstall.ps1`, `SHA256SUMS` 네 개. API digest·크기·다운로드 바이트 모두 일치 |
| 설치·업데이트·되돌림 | 신규 설치, v0.5.2 installer 업그레이드·되돌림, 이전 updater→latest, 두 번째 update 무교체 통과 |
| 구형 설치 | v0.3.5 updater는 자산 부재를 명시하고 원본 유지. 새 installer로 단일 바이너리 전환 통과 |
| 실제 설치본 | v0.5.3/동일 commit·digest. 없던 Clauduct settings 생성, `.old` 0개, 사용자 PATH 불변 |

실제 backend 총사용량은 개발 21회 + 출하 5회 = **26/30 attempts**다. 이 수치는 B의 별도 예산과 합치지 않는다.
`clauduct.exe` SHA-256은 `b4279f367e8d2b6ded220e50e4b76e42c79ad0c522a40db8b34243064b70675d`다.
[정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.3)에서 자산을 받는다.

Bundle C의 Workflow 확장·Office·최종 전체 기능 인수는 이 릴리스의 완료 항목이 아니다.
