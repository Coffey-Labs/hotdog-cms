import { useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react'

// The editor and the preview side by side, with a divider between them that
// can be dragged, or moved with the arrow keys, to suit the screen and the
// person. Double-click puts it back. The width is remembered per browser.
// On a narrow screen the panes stack and the divider goes away (styles.css).

const KEY = 'clcms-split'
const DEFAULT = 44
const MIN = 25
const MAX = 75
const clamp = (n: number) => Math.min(MAX, Math.max(MIN, Math.round(n * 10) / 10))

function stored(): number {
  try {
    const n = Number(localStorage.getItem(KEY))
    if (n >= MIN && n <= MAX) return n
  } catch {
    /* storage blocked: start at the default */
  }
  return DEFAULT
}

export function Split({ left, right }: { left: ReactNode; right: ReactNode }) {
  const [pct, setPct] = useState(stored)
  const [dragging, setDragging] = useState(false)
  const box = useRef<HTMLDivElement>(null)

  const save = (v: number) => {
    try {
      localStorage.setItem(KEY, String(v))
    } catch {
      /* not remembered, still works */
    }
  }
  const commit = (n: number) => {
    const v = clamp(n)
    setPct(v)
    save(v)
  }
  const fromPointer = (e: PointerEvent) => {
    const r = box.current?.getBoundingClientRect()
    if (r && r.width > 0) setPct(clamp(((e.clientX - r.left) / r.width) * 100))
  }
  const onKey = (e: KeyboardEvent) => {
    const step = e.shiftKey ? 10 : 2
    const next = ({ ArrowLeft: pct - step, ArrowRight: pct + step, Home: MIN, End: MAX } as Record<string, number>)[e.key]
    if (next === undefined) return
    e.preventDefault()
    commit(next)
  }

  return (
    <div className={`pageview${dragging ? ' resizing' : ''}`} ref={box} style={{ '--split': `${pct}%` } as CSSProperties}>
      {left}
      <div
        className="divider"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the editor and preview"
        aria-valuemin={MIN}
        aria-valuemax={MAX}
        aria-valuenow={Math.round(pct)}
        tabIndex={0}
        title="Drag to resize · double-click to reset"
        onPointerDown={(e) => {
          e.preventDefault()
          e.currentTarget.setPointerCapture(e.pointerId)
          setDragging(true)
        }}
        onPointerMove={(e) => dragging && fromPointer(e)}
        onPointerUp={(e) => {
          e.currentTarget.releasePointerCapture(e.pointerId)
          setDragging(false)
          save(pct)
        }}
        onPointerCancel={() => setDragging(false)}
        onDoubleClick={() => commit(DEFAULT)}
        onKeyDown={onKey}
      />
      {right}
    </div>
  )
}
