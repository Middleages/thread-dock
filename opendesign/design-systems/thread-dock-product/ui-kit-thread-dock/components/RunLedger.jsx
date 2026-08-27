export function RunLedger({ runs }) {
  return (
    <section aria-labelledby="run-ledger-title">
      <h1 id="run-ledger-title">오늘의 작업 원장</h1>
      <div role="table" aria-label="개발 작업">
        <div role="row" className="run header">
          <span role="columnheader">시각</span>
          <span role="columnheader">작업 번호</span>
          <span role="columnheader">작업</span>
          <span role="columnheader">상태</span>
        </div>
        {runs.map((run) => (
          <div role="row" className={run.current ? "run current" : "run"} key={run.id}>
            <time role="cell">{run.time}</time>
            <span role="cell">#{run.issue}</span>
            <span role="cell"><strong>{run.title}</strong><small>{run.detail}</small></span>
            <span role="cell"><i aria-hidden="true" className={`status ${run.tone}`} />{run.state}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
