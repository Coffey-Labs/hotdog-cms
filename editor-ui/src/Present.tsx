import { go } from './route'
import { DevicePicker, PreviewFrame, useDevice } from './Viewport'

// The preview on its own, filling the window: for showing a page to someone,
// at the size of the device they'll read it on.
export function Present({ id, url, branch }: { id: string; url: string; branch?: string }) {
  const [device, setDevice] = useDevice()
  return (
    <div className="present">
      <div className="preview-bar">
        <button type="button" className="ghost small" onClick={() => (history.length > 1 ? history.back() : go({ view: 'site', id, branch }))}>← Back to editing</button>
        <span className="spacer" />
        <DevicePicker device={device} onChange={setDevice} />
        <span className="spacer" />
        <a className="ghost small linkbtn" href={url} target="_blank" rel="noopener noreferrer">Open ↗</a>
      </div>
      <PreviewFrame src={url} device={device} title="Presentation preview" />
    </div>
  )
}
