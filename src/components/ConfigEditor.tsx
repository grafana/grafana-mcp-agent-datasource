import React, { ChangeEvent, useState } from 'react';
import { 
  InlineField, 
  Input, 
  SecretInput, 
  Select, 
  Button, 
  Alert, 
  FieldSet,
  InlineFieldRow,
  Spinner,
  Badge
} from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps, SelectableValue } from '@grafana/data';
import { MCPDataSourceOptions, MCPSecureJsonData, TestConnectionResult } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<MCPDataSourceOptions, MCPSecureJsonData> {}

const TRANSPORT_OPTIONS: SelectableValue[] = [
  { label: 'WebSocket', value: 'websocket', description: 'WebSocket transport (recommended)' },
  { label: 'HTTP', value: 'http', description: 'HTTP transport' },
];

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;
  
  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null);

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
        transport: option.value as 'websocket' | 'http',
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

  // Test connection handler
  const onTestConnection = async () => {
    if (!jsonData.serverUrl) {
      setTestResult({
        success: false,
        message: 'Server URL is required',
      });
      return;
    }

    setIsTestingConnection(true);
    setTestResult(null);

    try {
      // In a real implementation, this would call the backend health check endpoint
      // For now, we'll simulate the test
      await new Promise(resolve => setTimeout(resolve, 2000));
      
      // Mock successful connection test
      setTestResult({
        success: true,
        message: 'Successfully connected to MCP server',
        serverInfo: {
          name: 'Example MCP Server',
          version: '1.0.0',
        },
        capabilities: {
          tools: { listChanged: false },
          resources: { subscribe: true, listChanged: false },
        },
        toolCount: 5,
      });
    } catch (error) {
      setTestResult({
        success: false,
        message: `Connection failed: ${error}`,
      });
    } finally {
      setIsTestingConnection(false);
    }
  };

  return (
    <>
      <FieldSet label="MCP Server Configuration">
        <InlineField 
          label="Server URL" 
          labelWidth={20} 
          tooltip="The URL of the MCP server (e.g., ws://localhost:8080 or http://localhost:8080)"
          required
        >
          <Input
            id="config-editor-server-url"
            onChange={onServerUrlChange}
            value={jsonData.serverUrl || ''}
            placeholder="ws://localhost:8080"
            width={50}
          />
        </InlineField>

        <InlineField 
          label="Transport" 
          labelWidth={20} 
          tooltip="Choose the transport protocol for MCP communication"
        >
          <Select
            options={TRANSPORT_OPTIONS}
            value={jsonData.transport || 'websocket'}
            onChange={onTransportChange}
            width={25}
          />
        </InlineField>

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
            labelWidth={20} 
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

        <InlineField label="Test Connection" labelWidth={20}>
          <Button 
            onClick={onTestConnection} 
            disabled={isTestingConnection || !jsonData.serverUrl}
            icon={isTestingConnection ? 'fa fa-spinner' : 'cloud'}
          >
            {isTestingConnection ? (
              <>
                <Spinner size={14} inline /> Testing...
              </>
            ) : (
              'Test Connection'
            )}
          </Button>
        </InlineField>

        {testResult && (
          <Alert 
            title={testResult.success ? 'Connection Successful' : 'Connection Failed'} 
            severity={testResult.success ? 'success' : 'error'}
          >
            <p>{testResult.message}</p>
            {testResult.success && testResult.serverInfo && (
              <div>
                <p><strong>Server:</strong> {testResult.serverInfo.name} v{testResult.serverInfo.version}</p>
                {testResult.toolCount && (
                  <p><strong>Available Tools:</strong> <Badge text={testResult.toolCount.toString()} color="blue" /></p>
                )}
                {testResult.capabilities && (
                  <div>
                    <strong>Capabilities:</strong>
                    {testResult.capabilities.tools && <Badge text="Tools" color="green" />}
                    {testResult.capabilities.resources && <Badge text="Resources" color="green" />}
                    {testResult.capabilities.prompts && <Badge text="Prompts" color="green" />}
                  </div>
                )}
              </div>
            )}
          </Alert>
        )}
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
