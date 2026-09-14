import { useState } from 'react'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useUploadProjectFile } from '@/hooks/useProjectFiles'
import { UploadFileDialog } from './UploadFileDialog'
import { ProjectFileTree } from './ProjectFileTree'

interface Props {
  projectId: string
}

export function ProjectFilesPanel({ projectId }: Props) {
  const [uploadOpen, setUploadOpen] = useState(false)
  const uploadMutation = useUploadProjectFile(projectId)

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-4 py-2.5 border-b">
        <span className="text-sm font-medium text-muted-foreground">
          项目文件
        </span>
        <Button
          size="sm"
          className="h-8 text-xs gap-1"
          onClick={() => setUploadOpen(true)}
          disabled={uploadMutation.isPending}
        >
          <Upload className="size-3.5" />
          上传文件
        </Button>
      </div>

      {/* File tree */}
      <div className="flex-1 overflow-auto px-4 py-2">
        <ProjectFileTree projectId={projectId} />
      </div>

      <UploadFileDialog
        projectId={projectId}
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        onUpload={async (formData) => {
          await uploadMutation.mutateAsync(formData)
        }}
      />
    </div>
  )
}
