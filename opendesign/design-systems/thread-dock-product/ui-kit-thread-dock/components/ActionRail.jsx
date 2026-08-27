export function ActionRail({ action, onStop, githubUrl }) {
  return (
    <aside aria-labelledby="action-title">
      <h2 id="action-title">지금 필요한 행동</h2>
      <p>{action.summary}</p>
      <strong>{action.title}</strong>
      <p>{action.explanation}</p>
      <button type="button" onClick={onStop}>실행 중단</button>
      <a href={githubUrl}>GitHub에서 보기</a>
    </aside>
  );
}
