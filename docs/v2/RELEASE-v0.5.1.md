# v0.5.1 — 주입 감사, 내장 역할 env, Workflow·`/cd`·plugin 결함

Claude Code 2.1.283·Codex CLI 0.157.1 기준이다. 측정과 판정의 자세한 표는
[호환성 문서](COMPATIBILITY.md#v051--주입-감사-내장-역할-env-세션-중-cd-pluginmanaged)에 있다.

## 바뀐 것

- **요청에 더하던 문장을 줄였다(#144).** 고정 top-level `instructions`, 실패한 도구 결과 앞의
  `Tool execution failed:`, Agent·ToolSearch·Workflow 설명에 붙이던 행동 지시, 메뉴 역할의 prompt, Workflow worker
  지침, fork 재개 설명, 자식 취소 보충을 뺐다. 각 문장은 실제 backend에서 원래 문제 시나리오를 10회 돌려 재현이
  없을 때만 뺐다. 재현된 위임 영수증, inherit 메뉴의 model 요청과 별칭·isolation 문장, SendMessage 문장, 압축 효율
  지침은 유지했다. 측정에서 발동하지 않은 되찾은 보고의 "untrusted" 표시와 오류·결과 미확보 보충도 유지했다.
  로컬 토큰 계수 기본값은 13에서 1로 바뀌었다.
- **Codex 기준 0.157.1.** 과금 없는 재측정에서 CLI·plugin API 스냅샷과 exec 요청 구조가 0.157.0과 같았다.
- **내장 역할과 `CLAUDE_CODE_SUBAGENT_MODEL`(#145).** 이 값을 정했으면 model·effort 인자 없이 부른
  general-purpose·Explore·Plan은 native가 고른 모델·effort로 실행한다. 정하지 않았으면 기존 역할 표 그대로다.
- **Workflow.** 이름에 `:` 같은 문자가 있는 script(예: `clauduct:plan-v1`)의 자식이 모두 거부되던 결함, plan-v1에
  native가 무시하는 `description`·`title`이 붙으면 거부되던 결함, plan-v1을 `name` 필드로 부르면 거부되던 결함,
  `--resume`한 세션에서 이전 script를 다시 보내면 어댑터가 두 번 붙어 native가 거부하던 결함을 고쳤다.
- **세션 중 `/cd`(#146).** `/cd` 뒤 첫 프롬프트가 `CLAUDUCT_CONTEXT_SESSION_UNVERIFIED`로 막히던 결함을 고쳤다.
  native가 세션을 새 프로젝트 폴더로 옮긴 경우에만 따라가고 context 상태를 함께 옮긴다.
- **plugin(#148).** `.zip` `--plugin-dir`와 여러 plugin을 담은 `--plugin-dir` 폴더의 역할 호출이 거부되던 결함을
  고쳤다. managed 역할 경로는 native와 같게 읽음을 확인했다.
- **fork 자식(#147).** 측정만 했다. 재현된 결함은 없었다. 사용자가 UI에서 멈춘 fork 자식의 표시는 측정하지 않았다.

## 남은 것

- 두 유지 문장(inherit 메뉴의 model 요청, Agent의 별칭·isolation) 중 어느 쪽이 필요한지는 나눠 재지 않았다.
- managed `env`의 `CLAUDE_CODE_SUBAGENT_MODEL`은 여전히 거부하며 `managed-settings.d`와 registry 정책은 읽지 않는다.
- auto 모드 분류기는 v0.5.2(#149), Workflow 재실행 resume·Office·review-diff는 v0.6.0에서 다룬다.

## 출하 검사 (2026-09-26)

태그 `v0.5.1`은 `197ff48d8f68ddadb6698b44f53bac1e91a53666`이며 [GitHub Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.1)는
latest·정식 릴리스다. [PR #154](https://github.com/wotjr1649/Clauduct/pull/154).

| 검사 | 결과 |
|---|---|
| 재현 빌드 | 깨끗한 태그 트리에서 독립 캐시로 두 번 빌드해 바이트 동일, 신원 `v0.5.1`/`197ff48`, `+dirty` 없음. `clauduct.exe` SHA-256 `a5f572b6a6364cddadcaa2cd368bbf09053c9e0cfb10013a02bdecc73ef63371` |
| 설치 | 새 설치, v0.5.0 설치 위 교체, v0.5.0 설치 스크립트로 되돌리기, 사용자 PATH 불변 |
| 실제 backend 세션 | 입력·`/clear`·background 자식 위임 5회. 실패 0, 부모 luna/max·자식 luna/low |
| 실제 backend 기능 확인 | `CLAUDE_CODE_SUBAGENT_MODEL=gpt-6-sol`에서 모델 없는 general-purpose 자식이 native 선택 sol/low로 실행(4회). `.zip` `--plugin-dir` 역할이 정의대로 luna/medium(4회). 그 앞의 실행 하나는 하네스가 역할에 준 도구 제한 탓에 native가 자식을 시작하지 못해 실패했다(3회). 실제 TUI에서 `/cd`로 신뢰된 작업 공간의 하위 폴더로 옮긴 뒤 다음 프롬프트가 막히지 않고 답함(2회. 그 앞의 실행 하나는 하네스의 대기 조건 탓에 프롬프트를 보내지 못했다, 0회) |
| 발행 뒤 | latest·자산 4개·API digest·내려받은 바이트 대조, v0.5.0 updater의 `--update --yes`, 다음 실행의 `.old` 정리, 두 번째 `--update` 무변경, 0.3.5 설치의 updater가 새 릴리스를 거부하고 새 설치 스크립트가 전환 |
| 실제 설치본 | `clauduct --update --yes`로 v0.5.0 → v0.5.1, 같은 SHA-256, 두 번째 업데이트 무변경 |

로컬 검사는 gofmt, vet(기본·`policy_evidence`·`runtime_evidence`), build, 전체 테스트, race, 문서 인용 검사를 통과했다.
전체 race에서 app 패키지는 다른 패키지와 함께 돌 때 Go 기본 10분 제한을 넘겨 중단됐고, 단독 재실행(288초)과 해당
프로세스 트리 테스트 3회 반복은 통과했다.

v0.5.1 묶음의 실제 backend 사용은 사용자가 정한 총상한(450에서 800, 다시 1000으로 올림) 안에서 981회다. 주입 감사가
대부분을 썼다.
