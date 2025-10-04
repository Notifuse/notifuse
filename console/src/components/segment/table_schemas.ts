import { TableSchema } from './interfaces'
import { CountriesFormOptions, Timezones } from '../utils/countries_timezones'
import { Languages } from '../utils/languages'
import { faUser, faFolderOpen } from '@fortawesome/free-regular-svg-icons'
import { faMousePointer } from '@fortawesome/free-solid-svg-icons'

/**
 * Database table schemas for segmentation engine
 * Based on the actual database structure from internal/database/init.go
 */

export const ContactsTableSchema: TableSchema = {
  name: 'contacts',
  title: 'Contact property',
  description: 'Contact profile and custom fields',
  icon: faUser,
  fields: {
    email: {
      name: 'email',
      title: 'Email',
      description: 'Contact email address',
      type: 'string',
      shown: true
    },
    external_id: {
      name: 'external_id',
      title: 'External ID',
      description: 'External identifier from your system',
      type: 'string',
      shown: true
    },
    first_name: {
      name: 'first_name',
      title: 'First Name',
      description: 'Contact first name',
      type: 'string',
      shown: true
    },
    last_name: {
      name: 'last_name',
      title: 'Last Name',
      description: 'Contact last name',
      type: 'string',
      shown: true
    },
    phone: {
      name: 'phone',
      title: 'Phone',
      description: 'Contact phone number',
      type: 'string',
      shown: true
    },
    country: {
      name: 'country',
      title: 'Country',
      description: 'Contact country',
      type: 'string',
      shown: true,
      options: CountriesFormOptions
    },
    language: {
      name: 'language',
      title: 'Language',
      description: 'Contact language preference',
      type: 'string',
      shown: true,
      options: Languages.map((lang) => ({ value: lang.value, label: lang.name }))
    },
    timezone: {
      name: 'timezone',
      title: 'Timezone',
      description: 'Contact timezone',
      type: 'string',
      shown: true,
      options: Timezones.map((tz) => ({ value: tz.name, label: tz.name }))
    },
    address_line_1: {
      name: 'address_line_1',
      title: 'Address Line 1',
      description: 'Contact address line 1',
      type: 'string',
      shown: true
    },
    address_line_2: {
      name: 'address_line_2',
      title: 'Address Line 2',
      description: 'Contact address line 2',
      type: 'string',
      shown: false
    },
    postcode: {
      name: 'postcode',
      title: 'Postcode',
      description: 'Contact postal code',
      type: 'string',
      shown: true
    },
    state: {
      name: 'state',
      title: 'State',
      description: 'Contact state/province',
      type: 'string',
      shown: true
    },
    job_title: {
      name: 'job_title',
      title: 'Job Title',
      description: 'Contact job title',
      type: 'string',
      shown: true
    },
    lifetime_value: {
      name: 'lifetime_value',
      title: 'Lifetime Value',
      description: 'Customer lifetime value',
      type: 'number',
      shown: true
    },
    orders_count: {
      name: 'orders_count',
      title: 'Orders Count',
      description: 'Total number of orders',
      type: 'number',
      shown: true
    },
    last_order_at: {
      name: 'last_order_at',
      title: 'Last Order Date',
      description: 'Date of last order',
      type: 'time',
      shown: true
    },
    // Custom string fields
    custom_string_1: {
      name: 'custom_string_1',
      title: 'Custom String 1',
      description: 'Custom string field 1',
      type: 'string',
      shown: true
    },
    custom_string_2: {
      name: 'custom_string_2',
      title: 'Custom String 2',
      description: 'Custom string field 2',
      type: 'string',
      shown: true
    },
    custom_string_3: {
      name: 'custom_string_3',
      title: 'Custom String 3',
      description: 'Custom string field 3',
      type: 'string',
      shown: true
    },
    custom_string_4: {
      name: 'custom_string_4',
      title: 'Custom String 4',
      description: 'Custom string field 4',
      type: 'string',
      shown: true
    },
    custom_string_5: {
      name: 'custom_string_5',
      title: 'Custom String 5',
      description: 'Custom string field 5',
      type: 'string',
      shown: true
    },
    // Custom number fields
    custom_number_1: {
      name: 'custom_number_1',
      title: 'Custom Number 1',
      description: 'Custom number field 1',
      type: 'number',
      shown: true
    },
    custom_number_2: {
      name: 'custom_number_2',
      title: 'Custom Number 2',
      description: 'Custom number field 2',
      type: 'number',
      shown: true
    },
    custom_number_3: {
      name: 'custom_number_3',
      title: 'Custom Number 3',
      description: 'Custom number field 3',
      type: 'number',
      shown: true
    },
    custom_number_4: {
      name: 'custom_number_4',
      title: 'Custom Number 4',
      description: 'Custom number field 4',
      type: 'number',
      shown: true
    },
    custom_number_5: {
      name: 'custom_number_5',
      title: 'Custom Number 5',
      description: 'Custom number field 5',
      type: 'number',
      shown: true
    },
    // Custom datetime fields
    custom_datetime_1: {
      name: 'custom_datetime_1',
      title: 'Custom Date 1',
      description: 'Custom datetime field 1',
      type: 'time',
      shown: true
    },
    custom_datetime_2: {
      name: 'custom_datetime_2',
      title: 'Custom Date 2',
      description: 'Custom datetime field 2',
      type: 'time',
      shown: true
    },
    custom_datetime_3: {
      name: 'custom_datetime_3',
      title: 'Custom Date 3',
      description: 'Custom datetime field 3',
      type: 'time',
      shown: true
    },
    custom_datetime_4: {
      name: 'custom_datetime_4',
      title: 'Custom Date 4',
      description: 'Custom datetime field 4',
      type: 'time',
      shown: true
    },
    custom_datetime_5: {
      name: 'custom_datetime_5',
      title: 'Custom Date 5',
      description: 'Custom datetime field 5',
      type: 'time',
      shown: true
    },
    created_at: {
      name: 'created_at',
      title: 'Created At',
      description: 'Contact creation date',
      type: 'time',
      shown: true
    },
    updated_at: {
      name: 'updated_at',
      title: 'Updated At',
      description: 'Contact last update date',
      type: 'time',
      shown: false
    }
  }
}

export const ContactListsTableSchema: TableSchema = {
  name: 'contact_lists',
  title: 'List subscription',
  description: 'Contact list subscription status',
  icon: faFolderOpen,
  fields: {
    list_id: {
      name: 'list_id',
      title: 'List ID',
      description: 'List identifier',
      type: 'string',
      shown: true
    },
    status: {
      name: 'status',
      title: 'Status',
      description: 'Subscription status',
      type: 'string',
      shown: true,
      options: [
        { value: 'subscribed', label: 'Subscribed' },
        { value: 'unsubscribed', label: 'Unsubscribed' },
        { value: 'pending', label: 'Pending' },
        { value: 'bounced', label: 'Bounced' },
        { value: 'complained', label: 'Complained' }
      ]
    },
    created_at: {
      name: 'created_at',
      title: 'Subscribed At',
      description: 'Date when contact was added to list',
      type: 'time',
      shown: true
    },
    updated_at: {
      name: 'updated_at',
      title: 'Updated At',
      description: 'Last status update date',
      type: 'time',
      shown: false
    },
    deleted_at: {
      name: 'deleted_at',
      title: 'Deleted At',
      description: 'Date when contact was removed from list',
      type: 'time',
      shown: false
    }
  }
}

export const ContactTimelineTableSchema: TableSchema = {
  name: 'contact_timeline',
  title: 'Activity',
  description: 'Contact activity and change history',
  icon: faMousePointer,
  fields: {
    operation: {
      name: 'operation',
      title: 'Operation',
      description: 'Type of operation performed',
      type: 'string',
      shown: true,
      options: [
        { value: 'insert', label: 'Insert' },
        { value: 'update', label: 'Update' }
      ]
    },
    entity_type: {
      name: 'entity_type',
      title: 'Entity Type',
      description: 'Type of entity that changed',
      type: 'string',
      shown: true,
      options: [
        { value: 'contact', label: 'Contact' },
        { value: 'contact_list', label: 'Contact List' },
        { value: 'message_history', label: 'Message History' },
        { value: 'webhook_event', label: 'Webhook Event' }
      ]
    },
    entity_id: {
      name: 'entity_id',
      title: 'Entity ID',
      description: 'ID of the related entity',
      type: 'string',
      shown: true
    },
    created_at: {
      name: 'created_at',
      title: 'Event Date',
      description: 'When the event occurred',
      type: 'time',
      shown: true
    }
  }
}

// Export all schemas as a map
export const TableSchemas: { [key: string]: TableSchema } = {
  contacts: ContactsTableSchema,
  contact_lists: ContactListsTableSchema,
  contact_timeline: ContactTimelineTableSchema
}
