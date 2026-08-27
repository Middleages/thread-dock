export function WorkflowCatalog({ workflows, onDispatch }) {
  return (
    <section aria-labelledby="workflow-title">
      <h2 id="workflow-title">GitHub Actions</h2>
      {workflows.map((workflow) => (
        <article className="workflow" key={workflow.id}>
          <strong>{workflow.label}</strong>
          <span>{workflow.status}</span>
          <small>{workflow.schedule} · {workflow.updated}</small>
          {workflow.dispatchable ? (
            <button type="button" onClick={() => onDispatch(workflow.id)}>{workflow.actionLabel ?? "실행 내용 확인"}</button>
          ) : null}
        </article>
      ))}
    </section>
  );
}
