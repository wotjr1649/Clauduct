# v0.5.3 — Clauduct 설정과 세션 선택

2026-09-27 출하 후보. 제품 구현·로컬 검사·실제 backend 검증은 완료했으며 태그·Release·설치 검증은 아직 진행하지 않았다.

Clauduct의 모델 설정을 수동 `.clauduct/settings.json`에서 관리한다. 새 세션은 기본적으로 Sol/xhigh로 시작한다.
`startup`, GPT별 `modelDefaults`, Claude alias의 `modelMapping`, 개별 `agents`의 model·effort를 분리했다.
설정 형식과 우선순위는 [SETTINGS.md](SETTINGS.md)에 있다.

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

출하 후 깨끗한 태그의 재현 빌드·자산 digest·업데이트·설치 결과를 이 문서에 추가한다.
Bundle C의 Workflow 확장·Office·최종 전체 기능 인수는 이 릴리스의 완료 항목이 아니다.
