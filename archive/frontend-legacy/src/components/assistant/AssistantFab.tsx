import { useEffect } from 'react'
import { Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAssistantStore } from '@/stores/assistantStore'
import { AssistantPanel } from './AssistantPanel'

export function AssistantFab() {
  const { isOpen, toggle, fabVisibility } = useAssistantStore()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        toggle()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [toggle])

  return (
    <div
      className={`fixed bottom-6 right-6 z-50 flex flex-col items-end gap-3 transition-opacity ${
        fabVisibility === 'hidden' ? 'pointer-events-none opacity-0' : 'opacity-100'
      }`}
      aria-hidden={fabVisibility === 'hidden'}
    >
      {isOpen && <AssistantPanel />}
      <Button
        size="icon"
        className="size-12 rounded-full shadow-lg hover:shadow-xl transition-shadow"
        onClick={toggle}
      >
        <Sparkles className="size-5" />
      </Button>
    </div>
  )
}
