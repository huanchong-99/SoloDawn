import { describe, expect, it } from 'vitest';
import type { McpConfig } from 'shared/types';
import { buildMcpServersPayload } from '@/pages/ui-new/settings/McpSettingsNew';

const mcpConfigFixture: McpConfig = {
  servers: {},
  servers_path: ['mcpServers'],
  template: {
    mcpServers: {},
  },
  preconfigured: {},
  is_toml_config: false,
};

describe('buildMcpServersPayload', () => {
  it('returns empty object when editor content is blank', () => {
    const result = buildMcpServersPayload('   \n\t  ', mcpConfigFixture);
    expect(result).toEqual({});
  });

  it('extracts servers from full config when JSON is provided', () => {
    const result = buildMcpServersPayload(
      JSON.stringify({
        mcpServers: {
          github: {
            command: 'npx',
            args: ['-y', '@modelcontextprotocol/server-github'],
          },
        },
      }),
      mcpConfigFixture
    );

    expect(result).toEqual({
      github: {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-github'],
      },
    });
  });
});
