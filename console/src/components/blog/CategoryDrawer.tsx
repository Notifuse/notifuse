import { useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Button, Drawer, Form, Input, App } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { blogCategoriesApi, normalizeSlug, BlogCategory } from '../../services/api/blog'
import type { CreateBlogCategoryRequest, UpdateBlogCategoryRequest } from '../../services/api/blog'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'

const { TextArea } = Input

interface CategoryDrawerProps {
  open: boolean
  onClose: () => void
  category?: BlogCategory | null
  workspaceId: string
}

export function CategoryDrawer({ open, onClose, category, workspaceId }: CategoryDrawerProps) {
  const { t } = useLingui()
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
      message.success(t`Category created successfully`)
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      onClose()
      form.resetFields()
    },
    onError: (error: Error) => {
      message.error(t`Failed to create category: ${error.message}`)
    }
  })

  const updateMutation = useMutation({
    mutationFn: (data: UpdateBlogCategoryRequest) => blogCategoriesApi.update(workspaceId, data),
    onSuccess: () => {
      message.success(t`Category updated successfully`)
      queryClient.invalidateQueries({ queryKey: ['blog-categories', workspaceId] })
      onClose()
      form.resetFields()
    },
    onError: (error: Error) => {
      message.error(t`Failed to update category: ${error.message}`)
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

  const onFinish = (values: { name: string; slug: string; description?: string; seo?: Record<string, unknown> }) => {
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
      title={isEditMode ? t`Edit Category` : t`Create New Category`}
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
          {isEditMode ? t`Save` : t`Create`}
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
          label={t`Name`}
          rules={[
            { required: true, message: t`Please enter a category name` },
            { max: 255, message: t`Name must be less than 255 characters` }
          ]}
        >
          <Input placeholder={t`e.g., Product Updates`} onChange={handleNameChange} />
        </Form.Item>

        <Form.Item
          name="slug"
          label={t`Slug`}
          rules={[
            { required: true, message: t`Please enter a slug` },
            {
              pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
              message: t`Slug must contain only lowercase letters, numbers, and hyphens`
            },
            { max: 100, message: t`Slug must be less than 100 characters` }
          ]}
          extra={t`URL-friendly identifier (lowercase, hyphens only)`}
        >
          <Input placeholder="product-updates" disabled={isEditMode} />
        </Form.Item>

        <Form.Item name="description" label={t`Description`}>
          <TextArea
            rows={3}
            placeholder={t`Brief description of this category`}
            showCount
            maxLength={500}
          />
        </Form.Item>

        <SEOSettingsForm namePrefix={['seo']} />
      </Form>
    </Drawer>
  )
}
