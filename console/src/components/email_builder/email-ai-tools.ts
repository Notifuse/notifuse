import type { LLMTool } from '../../services/api/llm'
import type { EmailBlock, MJMLComponentType } from './types'

/**
 * Tool definitions for the Email AI Assistant
 * Follows the pattern from mjml_builder but adapted for Notifuse SSE streaming
 *
 * Key difference: No bidirectional tool results - tools are "fire and forget"
 * Tree context is injected in the system prompt instead of via getEmailStructure
 */

// Tool 1: updateBlock - Modify existing block
export const UPDATE_BLOCK_TOOL: LLMTool = {
  name: 'updateBlock',
  description:
    'Update the attributes or content of an existing MJML block. Use this to modify text content, styling, colors, fonts, padding, etc.',
  input_schema: {
    type: 'object',
    properties: {
      blockId: {
        type: 'string',
        description: 'The ID of the block to update (from the current email structure)'
      },
      updates: {
        type: 'object',
        description: 'An object containing the updates to apply',
        properties: {
          attributes: {
            type: 'object',
            description:
              'Styling attributes like backgroundColor, color, fontSize, align, paddingTop, paddingRight, paddingBottom, paddingLeft, borderRadius, width, href, src, alt, etc.'
          },
          content: {
            type: 'string',
            description: 'Text or HTML content for blocks like mj-text, mj-button, mj-title, mj-preview'
          }
        }
      }
    },
    required: ['blockId', 'updates']
  }
}

// Tool 2: addBlock - Add new block to parent
export const ADD_BLOCK_TOOL: LLMTool = {
  name: 'addBlock',
  description:
    'Add a new MJML block as a child of the specified parent. The new block will be created with default attributes.',
  input_schema: {
    type: 'object',
    properties: {
      parentId: {
        type: 'string',
        description: 'The ID of the parent block where this new block will be added'
      },
      blockType: {
        type: 'string',
        enum: [
          'mj-section',
          'mj-column',
          'mj-text',
          'mj-button',
          'mj-image',
          'mj-divider',
          'mj-spacer',
          'mj-social',
          'mj-social-element',
          'mj-wrapper',
          'mj-group',
          'mj-raw'
        ],
        description: 'The type of MJML block to add'
      },
      position: {
        type: 'number',
        description: 'Optional: The position index where to insert the block (0 = first). If omitted, appends to end.'
      },
      content: {
        type: 'string',
        description: 'Optional: Initial text/HTML content for mj-text, mj-button blocks'
      },
      attributes: {
        type: 'object',
        description: 'Optional: Initial attributes to set on the new block'
      }
    },
    required: ['parentId', 'blockType']
  }
}

// Tool 3: deleteBlock
export const DELETE_BLOCK_TOOL: LLMTool = {
  name: 'deleteBlock',
  description: 'Delete a block from the email template. This will also delete all child blocks.',
  input_schema: {
    type: 'object',
    properties: {
      blockId: {
        type: 'string',
        description: 'The ID of the block to delete'
      }
    },
    required: ['blockId']
  }
}

// Tool 4: moveBlock
export const MOVE_BLOCK_TOOL: LLMTool = {
  name: 'moveBlock',
  description: 'Move a block to a different parent or position within the email tree.',
  input_schema: {
    type: 'object',
    properties: {
      blockId: {
        type: 'string',
        description: 'The ID of the block to move'
      },
      newParentId: {
        type: 'string',
        description: 'The ID of the new parent block'
      },
      position: {
        type: 'number',
        description: 'The position index in the new parent (0 = first)'
      }
    },
    required: ['blockId', 'newParentId', 'position']
  }
}

// Tool 5: selectBlock - Highlight in editor
export const SELECT_BLOCK_TOOL: LLMTool = {
  name: 'selectBlock',
  description: 'Highlight and select a specific block in the visual editor so the user can see it.',
  input_schema: {
    type: 'object',
    properties: {
      blockId: {
        type: 'string',
        description: 'The ID of the block to select'
      }
    },
    required: ['blockId']
  }
}

// Tool 6: setEmailTree - Replace entire tree (for major changes)
export const SET_EMAIL_TREE_TOOL: LLMTool = {
  name: 'setEmailTree',
  description:
    'Replace the entire email structure with a new tree. Use this when building emails from scratch or making major structural changes. The tree must follow MJML hierarchy (mjml > mj-body > mj-section > mj-column > content).',
  input_schema: {
    type: 'object',
    properties: {
      tree: {
        type: 'object',
        description:
          'Complete email tree structure. Must have: id (string), type ("mjml"), and children array containing mj-body. Each block needs unique id and valid type.',
        properties: {
          id: { type: 'string', description: 'Unique identifier for the root block' },
          type: { type: 'string', enum: ['mjml'], description: 'Must be "mjml" for root' },
          children: {
            type: 'array',
            description: 'Array of child blocks (must include mj-body, optionally mj-head)'
          }
        },
        required: ['id', 'type', 'children']
      }
    },
    required: ['tree']
  }
}

// Tool 7: updateEmailMetadata - Subject/preview (Notifuse-specific)
export const UPDATE_EMAIL_METADATA_TOOL: LLMTool = {
  name: 'updateEmailMetadata',
  description: 'Update the email subject line and/or preview text.',
  input_schema: {
    type: 'object',
    properties: {
      subject: {
        type: 'string',
        description: 'The email subject line. Supports Liquid templating like {{ contact.first_name }}'
      },
      preview_text: {
        type: 'string',
        description: 'The preview text shown in email clients. Supports Liquid templating.'
      }
    },
    required: []
  }
}

/**
 * All tools for the Email AI Assistant
 */
export const EMAIL_AI_TOOLS: LLMTool[] = [
  UPDATE_BLOCK_TOOL,
  ADD_BLOCK_TOOL,
  DELETE_BLOCK_TOOL,
  MOVE_BLOCK_TOOL,
  SELECT_BLOCK_TOOL,
  SET_EMAIL_TREE_TOOL,
  UPDATE_EMAIL_METADATA_TOOL
]

/**
 * Tool names for type-safe event handling
 */
export const TOOL_NAMES = {
  UPDATE_BLOCK: 'updateBlock',
  ADD_BLOCK: 'addBlock',
  DELETE_BLOCK: 'deleteBlock',
  MOVE_BLOCK: 'moveBlock',
  SELECT_BLOCK: 'selectBlock',
  SET_EMAIL_TREE: 'setEmailTree',
  UPDATE_EMAIL_METADATA: 'updateEmailMetadata'
} as const

/**
 * Type for callbacks passed to the AI assistant
 */
export interface EmailAIAgentCallbacks {
  getEmailTree: () => EmailBlock
  setEmailTree: (tree: EmailBlock) => void
  onAddBlock: (parentId: string, blockType: MJMLComponentType, position?: number, content?: string, attributes?: Record<string, unknown>) => void
  onUpdateBlock: (blockId: string, updates: Partial<EmailBlock>) => void
  onDeleteBlock: (blockId: string) => void
  onMoveBlock: (blockId: string, newParentId: string, position: number) => void
  onSelectBlock: (blockId: string) => void
}

/**
 * Serialize the email tree for AI context
 * Simplifies the tree structure to reduce token usage while keeping essential info
 */
export function serializeEmailTree(tree: EmailBlock): string {
  const simplify = (block: EmailBlock, depth: number = 0): string => {
    const indent = '  '.repeat(depth)
    let result = `${indent}- ${block.type} (id: "${block.id}")`

    // Add full content for text blocks (HTML tags stripped for readability)
    if (block.content) {
      const plainText = block.content.replace(/<[^>]*>/g, '').trim()
      if (plainText) {
        result += ` content: "${plainText}"`
      }
    }

    // Add key attributes
    if (block.attributes) {
      const keyAttrs: string[] = []
      if (block.attributes.backgroundColor) keyAttrs.push(`bg: ${block.attributes.backgroundColor}`)
      if (block.attributes.color) keyAttrs.push(`color: ${block.attributes.color}`)
      if (block.attributes.width) keyAttrs.push(`width: ${block.attributes.width}`)
      if (block.attributes.href) keyAttrs.push(`href: ${block.attributes.href}`)
      if (block.attributes.src) keyAttrs.push(`src: ...`)
      if (keyAttrs.length > 0) {
        result += ` [${keyAttrs.join(', ')}]`
      }
    }

    // Recursively add children
    if (block.children && block.children.length > 0) {
      result += '\n' + block.children.map((child) => simplify(child, depth + 1)).join('\n')
    }

    return result
  }

  return simplify(tree)
}
