// Note: These validation messages are used as static strings in Form validation rules.
// Since they're used outside of React component context (in validation rules), they cannot
// use useLingui directly. These messages will be extracted by lingui extract and can be
// translated, but the translation needs to happen at the component level where possible.
// For Form validation messages, Ant Design displays these strings as-is.
// Consider creating a useMessages() hook for components that need translated validation messages.
const Messages = {
  RequiredField: 'This field is required',
  ServiceAccountPasswordInvalidFormat: 'The password should contain 16 charaters minimum.',
  EmailRequired: 'Please enter your email.',
  YourNameIsRequired: 'Please enter your name.',
  PasswordRequired: 'Please input your password.',
  NewPasswordInvalid: 'Your new password should contain at least 8 characters.',
  ConfirmPasswordRequired: 'Please confirm your new password.',
  PasswordsDontMatch: 'The two passwords that you entered do not match!',
  InvalidTimezone: 'This time zone is not valid.',
  InvalidIdFormat: 'The value should-be-written-like-this',
  InvalidTableName: 'The value should_be_written_like_this',
  InvalidWorkspaceIdFormat: 'The ID should only contain alphanumeric characters (az09)',
  ValidURLRequired: 'A valid URL is required',
  InvalidArrayOfStrings: 'The value should be an array of strings.',
  InvalidPath: 'The path should be like: /url-path',
  InvalidURLParamsFormat:
    'The parameter should only contain the following characters: A-Za-z0-9-_~',
  InvalidHostname: 'The value should be a valid host name.',
  InvalidFilterOperation: 'The operation is not valid.',
  InvalidTableColumName: 'The table column name should only contain the follow charats: a-Z0-9_-',
  InvalidOrganizationIDFormat: 'The ID should only contain alphanumeric characters (az09)'
}

export default Messages
