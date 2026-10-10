import { useEffect, useState } from 'react'
import { go } from './route'
import { DevicePicker, PreviewFrame, useDevice } from './Viewport'

// The preview on its own, filling the window: for showing a page to someone,
// at the size of the device they'll read it on.
export function Present({ id, url, branch }: { id: string; url: string; branch?: string }) {
  const [device, setDevice] = useDevice()
  // The editor tab says when the live preview re-renders, so this one shows
  // the edit as it's typed too.
  const [reload, setReload] = useState(0)
  useEffect(() => {
    const ch = liveChannel()
    if (!ch) return
    ch.onmessage = (e: MessageEvent<{ url?: string }>) => {
      if (e.data?.url === url) setReload((n) => n + 1)
    }
    return () => ch.close()
  }, [url])
  return (
    <div className="present">
      <div className="preview-bar">
        <button type="button" className="ghost small" onClick={() => (window.opener ? window.close() : history.length > 1 ? history.back() : go({ view: 'site', id, branch }))}>← Back to editing</button>
        <span className="spacer" />
        <DevicePicker device={device} onChange={setDevice} />
        <span className="spacer" />
        <a className="ghost small linkbtn" href={url} target="_blank" rel="noopener noreferrer">Open ↗</a>
      </div>
      <PreviewFrame src={url} device={device} title="Presentation preview" reloadKey={reload} />
    </div>
  )
}

// Between the editor's tabs only: a BroadcastChannel never leaves this origin.
export function liveChannel(): BroadcastChannel | undefined {
  return typeof BroadcastChannel === 'undefined' ? undefined : new BroadcastChannel('clcms-live')
}
