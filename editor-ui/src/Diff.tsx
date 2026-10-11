// A unified diff, shown the way git prints one: what goes, in red; what
// comes, in green; where, in the hunk lines between.
export function DiffView({ diff, empty = 'No changes.' }: { diff: string; empty?: string }) {
  if (!diff.trim()) return <p className="muted small">{empty}</p>
  return (
    <pre className="diff" aria-label="Changes">
      {diff.split('\n').map((l, i) => {
        const cls = l.startsWith('+++') || l.startsWith('---') || l.startsWith('diff ') || l.startsWith('index ') ? 'h' : l.startsWith('@@') ? 'at' : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : ''
        return <span key={i} className={cls}>{l + '\n'}</span>
      })}
    </pre>
  )
}
