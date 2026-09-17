# Session 31 — effort 상승 경로 추적과 지연 개선 여지

## 목표

Clauduct 요청이 기본 `low`가 아니라 `high` effort로 나가는 경로를 찾고, 그것이 의도인지 결함인지 판정한다. 그 다음 추론 시간을 줄일 수 있는지 판단한다. 판정과 근거를 문서로 남기는 것까지가 완료다.

## 지금 참인 것 (실측)

`D:\AIDEV\Clauduct\.clauduct-status\request-status.jsonl`에 과거 실행 기록이8개 snapshot으로 남아 있다. 그중 `recentRequests`가16개인 snapshot(2026-09-12T04:59:52,20 requests 전부 성공)을 단계별로 분해한 결과다.

| 구간 | 소요 | 비중 |
|---|---|---|
| 로컬 queue 대기 | 2.1ms | 0.0% |
| 로컬 준비 | 41.9ms | 0.0% |
| 서버 첫 응답 대기 | 18,816ms | 8.3% |
| 서버 streaming 수신 | 208,577ms | 91.7% |
| 마무리 | 7.0ms | 0.0% |

**Clauduct가 CPU를 쓰는 시간은 총51ms, 전체의0.02%다.** 다른 언어로 포팅해도 얻을 것이 없다. 이 결론은 확정이며 다시 검토하지 않는다.

`firstTextDeltaMs`로 streaming 구간을 다시 쪼개면 TTFT6.7% / **추론46.5%** / 텍스트 생성46.8%다. **단 이 분해는 `firstTextDeltaMs`가 기록된2건 기준이다(n=2).** 나머지14건은 해당 필드가 null이어서 제외됐다. 비율을 인용하기 전에 표본을 넓히거나 n=2임을 함께 적어야 한다.

실행된 모델과 effort는 `gpt-5.6-sol/high`14건, `gpt-5.6-luna/high`1건, `gpt-5.6-terra/high`1건이다. **16건 모두 `high`다.** 반면 `src/models.mjs:12`의 `DEFAULT_SELECTION`은 `astra` 모델에 effort `low`다.

## 다음 행동

1. **effort 결정 경로를 추적한다.** `src/models.mjs`의 `selectModel`, `src/agent-selection.mjs`, `src/compact-policy.mjs`를 읽는다. `compact-policy.mjs`에는 "일반 routing 이후 effort가 high/xhigh/max이면 medium으로 낮춘다"는 기존 서술이 있어(`docs/audit-2026-09-08.md:266` 참조) 조정 지점이 여럿일 수 있다.
2. **의도적 상승인지 결함인지 판정한다.** 요청자가 지정한 값인지, 라우팅이 올린 것인지, 기본값이 적용되지 않은 것인지 구분한다.
3. **개선 여지를 판단한다.** 낮은 effort가 맞는 작업에 high가 쓰이고 있다면 그것이 가장 큰 레버다. 공식 문서 근거: [Reasoning models](https://developers.openai.com/api/docs/guides/reasoning), [Extended thinking](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking) — 둘 다 낮은 effort가 지연과 비용을 줄인다고 명시한다.
4. 표본이 부족하면 새 실행으로 `firstTextDeltaMs`를 확보한다. **비용이 발생하므로 범위를 먼저 사용자와 합의한다.**

## 제약

- **live 실행은 실제 모델을 호출한다.** 1회 실측치: 6요청, 입력17,274 / 출력1,050 토큰, 무인425초 / 승인 대행26.6초.
- **무인 live 개발 모드는 구조적으로 통과할 수 없다.** 모델이 소스를 쓰면 `control/review.json`의 사전 승인 해시와 어긋나 이후 `run_tests`가 영구 대기한다. 승인을 기록하는 코드는 `verification/verify-native-development.mjs:601-604`의 `if (localNative)` 안에만 있다. 상세는 `docs/verification-modes-2026-09-14.md` 참조.
- **AdGuard가 켜져 있으면 loopback HTTP test5건이 실패한다.** `test-chat`, `test-http-close`, `test-native-gateway`, `checkExpectation`, `invalid-method`. 제품 결함이 아니다. 판별법과 근거는 `docs/native-startup-diagnostics-2026-09-14.md`에 있다.
- `verification/fixture-token-budget.mjs:13`이 `maxOutputTokens`를32768로 하드 제한한다. settings.json의64000은 적용되지 않으며 **이 상한을 완화해 통과시키지 않는다.**

## 저장소 상태

- main `5956453`, 로컬·원격 동기화 완료, CI 초록불
- CI는 `.github/workflows/tests.yml`에서111개 test file을 세 단계로 실행한다 — `node --test`(src), 개별 실행(verification·poc), PowerShell
- 미해결1건: `poc/test-claude-read-once.mjs`의 `read_extra_normal`이 CI에서만 실패한다. CI에서 제외돼 있고 근거·조건·되돌린 시도가 `docs/verification-modes-2026-09-14.md`에 기록돼 있다. **이번 작업 대상이 아니다.**

## 참조 문서

- `docs/verification-modes-2026-09-14.md` — 검증 모드 전수 실행, live 승인 게이트, managed 재현 절차
- `docs/tcp-shell-assessment-2026-09-14.md` — TCP 반닫기 원인 확정(loopback filter)
- `docs/local-use-release-decision.md` — 현재 출하 판정과 사용 범위
- `RELEASE.md` — 출하 이력과 알려진 제약

## 완료 기준

effort 상승 경로를 코드 근거(파일:라인)와 함께 특정하고, 의도/결함 여부를 판정하고, 개선 가능한 지점과 그 근거를 문서에 남긴다. 측정 없이 추정한 항목은 추정으로 표시한다.
