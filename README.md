# Blockstep

lct의 사용자 시나리오가 현재 패키지에서 어디까지 동작하고 어느 스텝에서 막혔는지, 막힌 원인 이슈를 누가 언제까지 처리하는지 한 곳에서 보여 주는 도구다. 플랫폼팀과 엔진팀이 함께 쓴다.

- 설계안: [docs/plan/blockstep-plan.md](docs/plan/blockstep-plan.md)
- UI 초안: Design 캔버스 「Blockstep UI 초안」 https://claude.ai/artifact/52dFYtU8Hw7SAFx28qNPo5 (공유된 사람만 열 수 있다)

## 구성

- Go `net/http`와 `html/template`로 서버에서 HTML을 만들고, 부분 갱신은 htmx로 한다
- 저장은 SQLite 파일 하나다
- 실행 파일 하나를 사내 서버에서 실행한다

## 줄끝

새로 만드는 파일은 LF로 쓴다. `.gitattributes`가 저장소에 담기는 쪽을 고정한다.
