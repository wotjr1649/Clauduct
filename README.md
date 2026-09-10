# Clauduct

Claude Code 인터페이스의 모델 요청을 로컬 게이트웨이를 통해 기존 Codex 로그인으로 라우팅합니다. Read/Edit/Bash/MCP 실행과 사용자 승인은 Claude Code가 담당합니다.

현재 추론 경로는 **Claude Code → Clauduct의 127.0.0.1 gateway → ChatGPT Codex backend 직접 HTTPS**입니다. Codex app-server를 호출하거나 실행 중인 Codex 앱의 세션에 요청을 전달하는 구현이 아닙니다. 로그인 자격 증명을 재사용하는 것과 Codex 앱의 실행 엔진·재시도·프로토콜 처리를 재사용하는 것은 다릅니다. 로컬 합성 테스트 통과는 실제 backend 오류가 발생하지 않는다는 보장이 아닙니다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd
D:\AIDEV\Clauduct\clauduct.cmd --model sol --effort xhigh
D:\AIDEV\Clauduct\clauduct.cmd --continue
D:\AIDEV\Clauduct\clauduct.cmd --resume <session-id>
```

메인 기본 실행은 **astra/low**입니다. 명시적 모델 선택의 기본값(astra/medium, sol/xhigh, terra/high, luna/max)과 고정 agent 정의는 보존합니다. `--effort`로 지정한 값이 우선합니다. 실행 중 `/model` 선택을 사용하며, 모델 미지정 Explore와 일반 역할은 luna/max, Plan은 sol/xhigh로 연결합니다. 자식 메타데이터와 부모 호출 ID로 확인한 명시 모델이 역할 기본값보다 우선합니다. 실제 실행에서 연결을 확인할 수 없으면 AGENT_SELECTION_UNVERIFIED로 중단하며 다른 모델로 추정 배정하지 않습니다.

- 텍스트는 생성 중 스트리밍하고, 도구 호출은 응답 완료 검증 뒤에 전달합니다.
- 기존 Codex가 갱신한 인증을 재읽습니다. 직접 OAuth 갱신이나 인증 파일 쓰기는 하지 않습니다.
- 메모리가 부족하면 새 요청을 대기시키며 진행 중 작업은 유지합니다.
- 응답 전달 전 일시 오류만 최대 5회 재시도합니다. 전달 후 실패는 명시적 재개가 필요합니다.
- 메인·서브에이전트에 400K 컨텍스트/320K 자동 압축 목표 정책을 전달합니다. 실제 backend 수용량과 실제 압축 시점은 별도 검증 대상입니다.

기존 native 설정 경로·플러그인·훅을 상속합니다. Clauduct용 라우팅 hook과 환경 설정은 자식 세션에만 추가합니다. 사용자가 해결한 모델 선택 keybindings와 전역 settings는 수정하지 않습니다. 보안상 provider/secret 계열 환경 변수는 자식에 복사하지 않으므로 이 값에 의존하는 도구는 일반 Claude 실행과 차이가 있습니다.

설치된 Node와 native Claude를 사용하는 Windows 실행기입니다. 사용자 PATH·프로필은 변경하지 않았으며, bare `clauduct` 등록 여부는 별개입니다. 인증 없이 실행 구성을 확인하려면 `clauduct.cmd --dry-run`을 사용합니다.

**Verified / Not verified의 상세 구분, 자원 제한, 호환성 제약과 검사 명령은 [native 구현 안내](docs/native.md)에 있습니다.** 기존 [Read 1회 PoC](poc/실제-Read-실행.md)의 사용자 SUCCESS와 이후 프롬프트 1~6 통과 보고는 보존하지만, 이번 스트리밍 변경의 실제 native 성공 증거로 대체하지 않습니다.
