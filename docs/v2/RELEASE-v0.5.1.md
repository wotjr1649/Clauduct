# v0.5.1 — 주입 감사, 내장 역할 env, Workflow·`/cd`·plugin 결함

Claude Code 2.1.283·Codex CLI 0.157.1 기준이다. 측정과 판정의 자세한 표는
[호환성 문서](COMPATIBILITY.md#v051--주입-감사-내장-역할-env-세션-중-cd-pluginmanaged)에 있다.

## 바뀐 것

- **요청에 더하던 문장을 줄였다(#144).** 고정 top-level `instructions`, 실패한 도구 결과 앞의
  `Tool execution failed:`, Agent·ToolSearch·Workflow 설명에 붙이던 행동 지시, 메뉴 역할의 prompt, Workflow worker
  지침, fork 재개 설명, 취소·실패·결과 미확보 보충을 뺐다. 각 문장은 실제 backend에서 원래 문제 시나리오를 10회
  돌려 재현이 없을 때만 뺐다. 재현된 위임 영수증, inherit 메뉴의 model 요청과 별칭·isolation 문장, SendMessage
  문장, 압축 효율 지침은 유지했다. 로컬 토큰 계수 기본값은 13에서 1로 바뀌었다.
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
