import React, { useState } from 'react'
import { BubbleMenu as TiptapBubbleMenu } from '@tiptap/react'
import { Editor } from '@tiptap/react'
import { Button, Input, Modal } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faBold,
  faItalic,
  faUnderline,
  faStrikethrough,
  faCode,
  faLink
} from '@fortawesome/free-solid-svg-icons'

interface BubbleMenuProps {
  editor: Editor
}

export const BubbleMenu: React.FC<BubbleMenuProps> = ({ editor }) => {
  const [isLinkModalOpen, setIsLinkModalOpen] = useState(false)
  const [linkUrl, setLinkUrl] = useState('')

  if (!editor) {
    return null
  }

  const handleOpenLinkModal = () => {
    const previousUrl = editor.getAttributes('link').href || ''
    setLinkUrl(previousUrl)
    setIsLinkModalOpen(true)
  }

  const handleSetLink = () => {
    if (linkUrl === '') {
      editor.chain().focus().extendMarkRange('link').unsetLink().run()
    } else {
      editor.chain().focus().extendMarkRange('link').setLink({ href: linkUrl }).run()
    }
    setIsLinkModalOpen(false)
    setLinkUrl('')
  }

  return (
    <>
      <TiptapBubbleMenu
        editor={editor}
        tippyOptions={{ 
          duration: 100,
          maxWidth: 'none',
          placement: 'top'
        }}
        className="bubble-menu"
      >
        <div className="bubble-menu-content">
          <Button
            size="small"
            type={editor.isActive('bold') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faBold} />}
            onClick={() => editor.chain().focus().toggleBold().run()}
            title="Bold (Ctrl+B)"
          />
          <Button
            size="small"
            type={editor.isActive('italic') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faItalic} />}
            onClick={() => editor.chain().focus().toggleItalic().run()}
            title="Italic (Ctrl+I)"
          />
          <Button
            size="small"
            type={editor.isActive('underline') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faUnderline} />}
            onClick={() => editor.chain().focus().toggleUnderline().run()}
            title="Underline (Ctrl+U)"
          />
          <Button
            size="small"
            type={editor.isActive('strike') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faStrikethrough} />}
            onClick={() => editor.chain().focus().toggleStrike().run()}
            title="Strikethrough"
          />
          <Button
            size="small"
            type={editor.isActive('code') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faCode} />}
            onClick={() => editor.chain().focus().toggleCode().run()}
            title="Code"
          />
          <div className="bubble-menu-divider" />
          <Button
            size="small"
            type={editor.isActive('link') ? 'primary' : 'text'}
            icon={<FontAwesomeIcon icon={faLink} />}
            onClick={handleOpenLinkModal}
            title="Link (Ctrl+K)"
          />
        </div>
      </TiptapBubbleMenu>

      <Modal
        title="Insert Link"
        open={isLinkModalOpen}
        onOk={handleSetLink}
        onCancel={() => {
          setIsLinkModalOpen(false)
          setLinkUrl('')
        }}
        okText="Insert"
      >
        <Input
          placeholder="https://example.com"
          value={linkUrl}
          onChange={(e) => setLinkUrl(e.target.value)}
          onPressEnter={handleSetLink}
          autoFocus
        />
      </Modal>
    </>
  )
}

