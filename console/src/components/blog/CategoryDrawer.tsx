import { useEffect } from 'react'
import { Button, Drawer, Form, Input, App } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { blogCategoriesApi, normalizeSlug, BlogCategory } from '../../services/api/blog.ts'
import type { CreateBlogCategoryRequest, UpdateBlogCategoryRequest } from '../../services/api/blog.ts'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'

const { TextArea } = Input

interface CategoryDrawerProps {
  open: boolean
  onClose: () => void
  category?: BlogCategory | null
  workspaceId: string
}

export function CategoryDrawer({ open, onClose, category, workspaceId }: CategoryDrawerProps) {
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const isEditMode = !!category

  useEffect(() => {
    if (open && category) {
      // Populate form with existing category data
      form.setFieldsValue({
        name: category.settings.name,
        slug: category.slug,
        description: category.settings.description,
        seo: category.settings.seo
      })
    } else if (open && !category) {
      form.resetFields()
    }
  }, [open, category, form])

  const createMutation = useMutation({
    mutationFn: (data: CreateBlogCategoryRequest) => blogCategoriesApi.create(workspaceId, data),
    onSuccess: () => {
      message.success('Category created successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      onClose()
      form.resetFields()
    },
    onError: (error: any) => {
      message.error(`Failed to create category: ${error.message}`)
    }
  })

  const updateMutation = useMutation({
    mutationFn: (data: UpdateBlogCategoryRequest) => blogCategoriesApi.update(workspaceId, data),
    onSuccess: () => {
      message.success('Category updated successfully')
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      onClose()
      form.resetFields()
    },
    onError: (error: any) => {
      message.error(`Failed to update category: ${error.message}`)
    }
  })

  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isEditMode) return // Don't update slug in edit mode

    const name = e.target.value
    const slug = normalizeSlug(name)
    form.setFieldsValue({ slug })
  }

  const handleClose = () => {
    onClose()
    form.resetFields()
  }

  const onFinish = (values: any) => {
    if (isEditMode && category) {
      const request: UpdateBlogCategoryRequest = {
        id: category.id,
        name: values.name,
        slug: values.slug,
        description: values.description,
        seo: values.seo
      }
      updateMutation.mutate(request)
    } else {
      const request: CreateBlogCategoryRequest = {
        name: values.name,
        slug: values.slug,
        description: values.description,
        seo: values.seo
      }
      createMutation.mutate(request)
    }
  }

  return (
    <Drawer
      title={isEditMode ? 'Edit Category' : 'Create New Category'}
      width={500}
      onClose={handleClose}
      open={open}
      styles={{
        body: { paddingBottom: 80 }
      }}
      extra={
        <Button
          type="primary"
          onClick={() => form.submit()}
          loading={isEditMode ? updateMutation.isPending : createMutation.isPending}
        >
          {isEditMode ? 'Save' : 'Create'}
        </Button>
      }
    >
      <Form
        form={form}
        layout="vertical"
        onFinish={onFinish}
        initialValues={{
          name: '',
          slug: '',
          description: ''
        }}
      >
        <Form.Item
          name="name"
          label="Name"
          rules={[
            { required: true, message: 'Please enter a category name' },
            { max: 255, message: 'Name must be less than 255 characters' }
          ]}
        >
          <Input placeholder="e.g., Product Updates" onChange={handleNameChange} />
        </Form.Item>

        <Form.Item
          name="slug"
          label="Slug"
          rules={[
            { required: true, message: 'Please enter a slug' },
            {
              pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
              message: 'Slug must contain only lowercase letters, numbers, and hyphens'
            },
            { max: 100, message: 'Slug must be less than 100 characters' }
          ]}
          extra="URL-friendly identifier (lowercase, hyphens only)"
        >
          <Input placeholder="product-updates" disabled={isEditMode} />
        </Form.Item>

        <Form.Item name="description" label="Description">
          <TextArea
            rows={3}
            placeholder="Brief description of this category"
            showCount
            maxLength={500}
          />
        </Form.Item>

        <SEOSettingsForm />
      </Form>
    </Drawer>
  )
}
