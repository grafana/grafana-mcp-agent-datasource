import React, { ChangeEvent } from 'react';
import {
  InlineField,
  Input,
  SecretInput,
  Select,
  FieldSet,
  InlineFieldRow
} from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps, SelectableValue } from '@grafana/data';
import { MCPDataSourceOptions, MCPSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<MCPDataSourceOptions, MCPSecureJsonData> { }

const TRANSPORT_OPTIONS: SelectableValue[] = [
  { label: 'Stream', value: 'stream', description: 'Streamable HTTP transport (recommended, uses configurable path)' },
  { label: 'SSE', value: 'sse', description: 'Server-Sent Events transport (deprecated, uses /sse)' },
];

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  // Handler for server URL changes
  const onServerUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        serverUrl: event.target.value,
      },
    });
  };

  // Handler for transport protocol changes
  const onTransportChange = (option: SelectableValue<string>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        transport: option.value as 'stream' | 'sse',
      },
    });
  };

  // Handler for stream path changes
  const onStreamPathChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        streamPath: event.target.value,
      },
    });
  };



  // Handler for timeout changes
  const onTimeoutChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        timeout: isNaN(value) ? undefined : value,
      },
    });
  };

  // Handler for max retries changes
  const onMaxRetriesChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        maxRetries: isNaN(value) ? undefined : value,
      },
    });
  };

  // Handler for retry interval changes
  const onRetryIntervalChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        retryInterval: isNaN(value) ? undefined : value,
      },
    });
  };

  // Secure field handlers
  const onAPIKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        apiKey: event.target.value,
      },
    });
  };

  const onAuthTokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        authToken: event.target.value,
      },
    });
  };

  const onUsernameChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        username: event.target.value,
      },
    });
  };

  const onPasswordChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        password: event.target.value,
      },
    });
  };

  const onClientIdChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        clientId: event.target.value,
      },
    });
  };

  const onClientSecretChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        clientSecret: event.target.value,
      },
    });
  };

  // Reset handlers for secure fields
  const onResetAPIKey = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        apiKey: false,
      },
      secureJsonData: {
        ...secureJsonData,
        apiKey: '',
      },
    });
  };

  const onResetAuthToken = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        authToken: false,
      },
      secureJsonData: {
        ...secureJsonData,
        authToken: '',
      },
    });
  };

  const onResetUsername = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        username: false,
      },
      secureJsonData: {
        ...secureJsonData,
        username: '',
      },
    });
  };

  const onResetPassword = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        password: false,
      },
      secureJsonData: {
        ...secureJsonData,
        password: '',
      },
    });
  };

  const onResetClientId = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        clientId: false,
      },
      secureJsonData: {
        ...secureJsonData,
        clientId: '',
      },
    });
  };

  const onResetClientSecret = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        clientSecret: false,
      },
      secureJsonData: {
        ...secureJsonData,
        clientSecret: '',
      },
    });
  };



  return (
    <>
      <FieldSet label="MCP Server Configuration">
        <InlineField
          label="Server URL"
          labelWidth={20}
          tooltip="The HTTP/HTTPS URL of the MCP server base URL"
          required
        >
          <Input
            id="config-editor-server-url"
            onChange={onServerUrlChange}
            value={jsonData.serverUrl || ''}
            placeholder="http://localhost:8080"
            width={50}
          />
        </InlineField>

        <InlineFieldRow>
          <InlineField
            label="Transport"
            labelWidth={20}
            tooltip="Choose the transport protocol. Stream is recommended as SSE is deprecated in the MCP spec."
          >
            <Select
              options={TRANSPORT_OPTIONS}
              value={jsonData.transport || 'stream'}
              onChange={onTransportChange}
              width={25}
            />
          </InlineField>

          {(jsonData.transport || 'stream') === 'stream' && (
            <InlineField
              label="Stream Path"
              labelWidth={20}
              tooltip="The path for the stream transport endpoint (e.g., /stream, /mcp)"
            >
              <Input
                id="config-editor-stream-path"
                onChange={onStreamPathChange}
                value={jsonData.streamPath || '/stream'}
                placeholder="/stream"
                width={25}
              />
            </InlineField>
          )}
        </InlineFieldRow>

        <InlineFieldRow>
          <InlineField
            label="Timeout (seconds)"
            labelWidth={20}
            tooltip="Request timeout in seconds"
          >
            <Input
              id="config-editor-timeout"
              type="number"
              onChange={onTimeoutChange}
              value={jsonData.timeout || 30}
              placeholder="30"
              width={15}
              min={1}
              max={300}
            />
          </InlineField>

          <InlineField
            label="Max Retries"
            labelWidth={20}
            tooltip="Maximum number of retry attempts"
          >
            <Input
              id="config-editor-max-retries"
              type="number"
              onChange={onMaxRetriesChange}
              value={jsonData.maxRetries || 3}
              placeholder="3"
              width={15}
              min={0}
              max={10}
            />
          </InlineField>

          <InlineField
            label="Retry Interval (seconds)"
            labelWidth={24}
            tooltip="Time to wait between retry attempts"
          >
            <Input
              id="config-editor-retry-interval"
              type="number"
              onChange={onRetryIntervalChange}
              value={jsonData.retryInterval || 5}
              placeholder="5"
              width={15}
              min={1}
              max={60}
            />
          </InlineField>
        </InlineFieldRow>
      </FieldSet>

      <FieldSet label="Authentication (Optional)">
        <InlineField
          label="API Key"
          labelWidth={20}
          tooltip="API key for authentication"
        >
          <SecretInput
            id="config-editor-api-key"
            isConfigured={secureJsonFields?.apiKey}
            value={secureJsonData?.apiKey || ''}
            placeholder="Enter your API key"
            width={40}
            onReset={onResetAPIKey}
            onChange={onAPIKeyChange}
          />
        </InlineField>

        <InlineField
          label="Auth Token"
          labelWidth={20}
          tooltip="Bearer token for authentication"
        >
          <SecretInput
            id="config-editor-auth-token"
            isConfigured={secureJsonFields?.authToken}
            value={secureJsonData?.authToken || ''}
            placeholder="Enter your auth token"
            width={40}
            onReset={onResetAuthToken}
            onChange={onAuthTokenChange}
          />
        </InlineField>

        <InlineFieldRow>
          <InlineField
            label="Username"
            labelWidth={20}
            tooltip="Username for basic authentication"
          >
            <SecretInput
              id="config-editor-username"
              isConfigured={secureJsonFields?.username}
              value={secureJsonData?.username || ''}
              placeholder="Username"
              width={25}
              onReset={onResetUsername}
              onChange={onUsernameChange}
            />
          </InlineField>

          <InlineField
            label="Password"
            labelWidth={20}
            tooltip="Password for basic authentication"
          >
            <SecretInput
              id="config-editor-password"
              isConfigured={secureJsonFields?.password}
              value={secureJsonData?.password || ''}
              placeholder="Password"
              width={25}
              onReset={onResetPassword}
              onChange={onPasswordChange}
            />
          </InlineField>
        </InlineFieldRow>

        <InlineFieldRow>
          <InlineField
            label="Client ID"
            labelWidth={20}
            tooltip="OAuth2 client ID"
          >
            <SecretInput
              id="config-editor-client-id"
              isConfigured={secureJsonFields?.clientId}
              value={secureJsonData?.clientId || ''}
              placeholder="Client ID"
              width={25}
              onReset={onResetClientId}
              onChange={onClientIdChange}
            />
          </InlineField>

          <InlineField
            label="Client Secret"
            labelWidth={20}
            tooltip="OAuth2 client secret"
          >
            <SecretInput
              id="config-editor-client-secret"
              isConfigured={secureJsonFields?.clientSecret}
              value={secureJsonData?.clientSecret || ''}
              placeholder="Client Secret"
              width={25}
              onReset={onResetClientSecret}
              onChange={onClientSecretChange}
            />
          </InlineField>
        </InlineFieldRow>
      </FieldSet>
    </>
  );
}
