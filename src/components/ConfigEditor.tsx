import React, { ChangeEvent, useState, useEffect } from 'react';
import {
  InlineField,
  Input,
  SecretInput,
  Select,
  FieldSet,
  InlineFieldRow,
  Button,
  Checkbox,
  IconButton,
  TextArea
} from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps, SelectableValue } from '@grafana/data';
import { MCPDataSourceOptions, MCPSecureJsonData, MCPArgument } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<MCPDataSourceOptions, MCPSecureJsonData> { }

const TRANSPORT_OPTIONS: SelectableValue[] = [
  { label: 'Stream', value: 'stream', description: 'Streamable HTTP transport (recommended, uses configurable path)' },
  { label: 'SSE', value: 'sse', description: 'Server-Sent Events transport (deprecated, uses /sse)' },
];

const LLM_PROVIDER_OPTIONS: SelectableValue[] = [
  { label: 'Mock Provider', value: 'mock', description: 'Mock LLM for testing (no API key required)' },
  { label: 'Anthropic Claude', value: 'anthropic', description: 'Anthropic Claude API for intelligent queries' },
  { label: 'OpenAI GPT', value: 'openai', description: 'OpenAI GPT API for intelligent queries' },
];

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  // State for managing arguments in the UI
  const [arguments_, setArguments] = useState<MCPArgument[]>([]);

  // Initialize arguments from stored configuration
  useEffect(() => {
    const regularArgs = jsonData.arguments || {};
    const secureArgNames = jsonData.secureArguments || [];
    
    const allArgs: MCPArgument[] = [];
    
    // Add regular arguments
    Object.entries(regularArgs).forEach(([key, value]) => {
      allArgs.push({
        key,
        value,
        isSecure: false,
      });
    });
    
    // Add secure arguments
    secureArgNames.forEach((key) => {
      const secureKey = `arg_${key}`;
      const isConfigured = secureJsonFields?.[secureKey] || false;
      const value = isConfigured ? '***' : (secureJsonData?.[secureKey] || '');
      
      allArgs.push({
        key,
        value,
        isSecure: true,
      });
    });
    
    setArguments(allArgs);
  }, [jsonData.arguments, jsonData.secureArguments, secureJsonFields, secureJsonData]);

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

  // Argument management handlers
  const updateStoredArguments = (newArgs: MCPArgument[]) => {
    const regularArgs: Record<string, string> = {};
    const secureArgNames: string[] = [];
    const newSecureJsonData = { ...secureJsonData };
    const newSecureJsonFields = { ...secureJsonFields };

    newArgs.forEach((arg) => {
      if (arg.isSecure) {
        secureArgNames.push(arg.key);
        const secureKey = `arg_${arg.key}`;
        if (arg.value && arg.value !== '***') {
          newSecureJsonData[secureKey] = arg.value;
        }
      } else {
        regularArgs[arg.key] = arg.value;
      }
    });

    // Clean up old secure arguments that are no longer present
    Object.keys(secureJsonData || {}).forEach((key) => {
      if (key.startsWith('arg_') && key !== 'llmApiKey') {
        const argName = key.substring(4); // Remove 'arg_' prefix
        if (!secureArgNames.includes(argName)) {
          delete newSecureJsonData[key];
          delete newSecureJsonFields[key];
        }
      }
    });

    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        arguments: regularArgs,
        secureArguments: secureArgNames,
      },
      secureJsonData: newSecureJsonData,
      secureJsonFields: newSecureJsonFields,
    });
  };

  const onAddArgument = () => {
    const newArg: MCPArgument = {
      key: '',
      value: '',
      isSecure: false,
      isNew: true,
    };
    setArguments([...arguments_, newArg]);
  };

  const onRemoveArgument = (index: number) => {
    const newArgs = arguments_.filter((_, i) => i !== index);
    setArguments(newArgs);
    updateStoredArguments(newArgs);
  };

  const onArgumentKeyChange = (index: number, key: string) => {
    const newArgs = [...arguments_];
    newArgs[index] = { ...newArgs[index], key };
    setArguments(newArgs);
    
    if (!newArgs[index].isNew) {
      updateStoredArguments(newArgs);
    }
  };

  const onArgumentValueChange = (index: number, value: string) => {
    const newArgs = [...arguments_];
    newArgs[index] = { ...newArgs[index], value };
    setArguments(newArgs);
    
    if (!newArgs[index].isNew) {
      updateStoredArguments(newArgs);
    }
  };

  const onArgumentSecureToggle = (index: number) => {
    const newArgs = [...arguments_];
    newArgs[index] = { ...newArgs[index], isSecure: !newArgs[index].isSecure };
    setArguments(newArgs);
    
    if (!newArgs[index].isNew) {
      updateStoredArguments(newArgs);
    }
  };

  const onSaveNewArgument = (index: number) => {
    const newArgs = [...arguments_];
    newArgs[index] = { ...newArgs[index], isNew: false };
    setArguments(newArgs);
    updateStoredArguments(newArgs);
  };

  const onResetSecureArgument = (argKey: string) => {
    const secureKey = `arg_${argKey}`;
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        [secureKey]: false,
      },
      secureJsonData: {
        ...secureJsonData,
        [secureKey]: '',
      },
    });
  };

  // LLM Configuration handlers
  const onLLMProviderChange = (option: SelectableValue<string>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        llmProvider: option.value as 'anthropic' | 'openai' | 'mock',
      },
    });
  };

  const onLLMModelChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        llmModel: event.target.value,
      },
    });
  };

  const onLLMAPIKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...secureJsonData,
        llmApiKey: event.target.value,
      },
    });
  };

  const onResetLLMAPIKey = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...secureJsonFields,
        llmApiKey: false,
      },
      secureJsonData: {
        ...secureJsonData,
        llmApiKey: '',
      },
    });
  };

  const onSystemPromptChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        systemPrompt: event.target.value,
      },
    });
  };

  const onMaxTokensChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        maxTokens: isNaN(value) ? undefined : value,
      },
    });
  };

  const onAgentRetriesChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        agentRetries: isNaN(value) ? undefined : value,
      },
    });
  };

  return (
    <div className="gf-form-group">
      <FieldSet label="Server Connection">
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

      <FieldSet label="Arguments">
        <div style={{ marginBottom: '16px' }}>
          <div style={{ fontSize: '12px', color: '#6e6e6e', marginBottom: '8px' }}>
            Configure key-value arguments to pass to the MCP server. Use secure arguments for sensitive data like passwords, connection strings, or API keys.
          </div>
          
          {arguments_.map((arg, index) => (
            <div key={index} style={{ display: 'flex', alignItems: 'center', marginBottom: '8px', gap: '8px' }}>
              <Input
                placeholder="Argument key (e.g., host, database)"
                value={arg.key}
                onChange={(e: ChangeEvent<HTMLInputElement>) => onArgumentKeyChange(index, e.target.value)}
                width={20}
              />
              
              {arg.isSecure ? (
                <SecretInput
                  placeholder="Secure value"
                  value={arg.value}
                  isConfigured={secureJsonFields?.[`arg_${arg.key}`] || false}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => onArgumentValueChange(index, e.target.value)}
                  onReset={() => onResetSecureArgument(arg.key)}
                  width={30}
                />
              ) : (
                <Input
                  placeholder="Value"
                  value={arg.value}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => onArgumentValueChange(index, e.target.value)}
                  width={30}
                />
              )}
              
              {arg.isNew && (
                <Checkbox
                  label="Secure"
                  value={arg.isSecure}
                  onChange={() => onArgumentSecureToggle(index)}
                />
              )}
              
              {!arg.isNew && arg.isSecure && (
                <div style={{ display: 'flex', alignItems: 'center', minWidth: '60px', fontSize: '12px', color: '#6e6e6e' }}>
                  🔒 Secure
                </div>
              )}
              
              {!arg.isNew && !arg.isSecure && (
                <div style={{ minWidth: '60px' }}></div>
              )}
              
              {arg.isNew ? (
                <Button
                  size="sm"
                  variant="primary"
                  onClick={() => onSaveNewArgument(index)}
                  disabled={!arg.key.trim()}
                >
                  Save
                </Button>
              ) : (
                <IconButton
                  name="trash-alt"
                  size="sm"
                  onClick={() => onRemoveArgument(index)}
                  tooltip="Remove argument"
                />
              )}
            </div>
          ))}
          
          <Button size="sm" variant="secondary" onClick={onAddArgument}>
            Add Argument
          </Button>
        </div>
      </FieldSet>

      <FieldSet label="Agent Configuration">
        <InlineField
          label="LLM Provider"
          labelWidth={20}
          tooltip="Choose the LLM provider for natural language query processing"
        >
          <Select
            options={LLM_PROVIDER_OPTIONS}
            value={jsonData.llmProvider || 'mock'}
            onChange={onLLMProviderChange}
            width={30}
          />
        </InlineField>

        <InlineField
          label="LLM Model"
          labelWidth={20}
          tooltip="The specific model to use (e.g., claude-3-5-sonnet-20241022, gpt-4)"
        >
          <Input
            id="config-editor-llm-model"
            onChange={onLLMModelChange}
            value={jsonData.llmModel || ''}
            placeholder={(jsonData.llmProvider === 'anthropic') ? 'claude-3-5-sonnet-20241022' : 
                         (jsonData.llmProvider === 'openai') ? 'gpt-4' : 'mock-model'}
            width={40}
          />
        </InlineField>

        {(jsonData.llmProvider === 'anthropic' || jsonData.llmProvider === 'openai') && (
          <InlineField
            label="LLM API Key"
            labelWidth={20}
            tooltip={`API key for ${jsonData.llmProvider === 'anthropic' ? 'Anthropic Claude' : 'OpenAI GPT'} service`}
          >
            <SecretInput
              id="config-editor-llm-api-key"
              isConfigured={secureJsonFields?.llmApiKey}
              value={secureJsonData?.llmApiKey || ''}
              placeholder={`Enter your ${jsonData.llmProvider === 'anthropic' ? 'Anthropic' : 'OpenAI'} API key`}
              width={40}
              onReset={onResetLLMAPIKey}
              onChange={onLLMAPIKeyChange}
            />
          </InlineField>
        )}

        <InlineField
          label="System Prompt"
          labelWidth={20}
          tooltip="System prompt that will always be sent to the LLM to set context and behavior"
        >
          <TextArea
            id="config-editor-system-prompt"
            onChange={onSystemPromptChange}
            value={jsonData.systemPrompt || ''}
            placeholder="You are an intelligent agent that helps users query and analyze data using available tools. Be helpful, accurate, and concise in your responses."
            rows={3}
            cols={60}
          />
        </InlineField>

        <InlineField
          label="Max Tokens"
          labelWidth={20}
          tooltip="Maximum number of tokens the LLM can generate in a single response (1-4000)"
        >
          <Input
            id="config-editor-max-tokens"
            type="number"
            onChange={onMaxTokensChange}
            value={jsonData.maxTokens || 1000}
            placeholder="1000"
            width={15}
            min={1}
            max={4000}
          />
        </InlineField>

        <InlineField
          label="Agent Retries"
          labelWidth={20}
          tooltip="Number of retry attempts when agent calls fail due to syntax errors or other issues"
        >
          <Input
            id="config-editor-agent-retries"
            type="number"
            onChange={onAgentRetriesChange}
            value={jsonData.agentRetries || 5}
            placeholder="5"
            width={15}
            min={1}
            max={10}
          />
        </InlineField>

        {jsonData.llmProvider === 'mock' && (
          <div style={{ marginTop: '8px', padding: '8px', backgroundColor: '#f0f0f0', borderRadius: '4px', fontSize: '12px' }}>
            <strong>Mock Provider:</strong> No API key required. This provider generates simple responses for testing purposes.
          </div>
        )}
      </FieldSet>
    </div>
  );
}
