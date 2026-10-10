import { useEffect, useRef, useState } from 'react'

// The preview, at the size of the device it's being judged on. Desktop is
// drawn at a real desktop width and scaled down to fit the pane; tablet and
// mobile are drawn at their own size, scaled only if the pane is smaller.
// The choice is remembered per browser.

export type Device = 'desktop' | 'tablet' | 'mobile'
const SIZES: Record<Device, { w: number; h: number; label: string }> = {
  desktop: { w: 1280, h: 800, label: 'Desktop' },
  tablet: { w: 768, h: 1024, label: 'Tablet' },
  mobile: { w: 390, h: 844, label: 'Mobile' },
}

export function useDevice(): [Device, (d: Device) => void] {
  const [d, setD] = useState<Device>(() => {
    try {
      const v = localStorage.getItem('hotdog-device')
      if (v === 'desktop' || v === 'tablet' || v === 'mobile') return v
    } catch {
      /* storage blocked: start on desktop */
    }
    return 'desktop'
  })
  const set = (v: Device) => {
    setD(v)
    try {
      localStorage.setItem('hotdog-device', v)
    } catch {
      /* not remembered, still works */
    }
  }
  return [d, set]
}

export function DevicePicker({ device, onChange }: { device: Device; onChange: (d: Device) => void }) {
  return (
    <div className="seg" role="group" aria-label="Preview size">
      {(Object.keys(SIZES) as Device[]).map((d) => (
        <button key={d} type="button" aria-pressed={device === d} onClick={() => onChange(d)} title={`${SIZES[d].label}, ${SIZES[d].w}×${SIZES[d].h}`}>
          {SIZES[d].label}
        </button>
      ))}
    </div>
  )
}

export function PreviewFrame({ src, device, title, reloadKey }: { src?: string; device: Device; title: string; reloadKey?: number }) {
  const box = useRef<HTMLDivElement>(null)
  const [space, setSpace] = useState({ w: 800, h: 600 })
  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setSpace({ w: e.contentRect.width, h: e.contentRect.height }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const size = SIZES[device]
  const pad = 24
  const availW = Math.max(100, space.w - pad * 2)
  const availH = Math.max(100, space.h - pad * 2)
  const scale = device === 'desktop' ? Math.min(1, availW / size.w) : Math.min(1, availW / size.w, availH / size.h)
  const h = device === 'desktop' ? availH / scale : size.h
  return (
    <div className={`frame-box ${device}`} ref={box}>
      {src ? (
        <div className="device" style={{ width: size.w * scale, height: h * scale }}>
          <iframe
            key={reloadKey}
            title={title}
            src={src}
            sandbox="allow-same-origin allow-scripts allow-popups allow-forms"
            style={{ width: size.w, height: h, transform: `scale(${scale})` }}
          />
        </div>
      ) : (
        <p className="muted pad">No preview yet.</p>
      )}
      <span className="frame-size muted small">{size.label} · {size.w}px{scale < 1 ? ` · shown at ${Math.round(scale * 100)}%` : ''}</span>
    </div>
  )
}
