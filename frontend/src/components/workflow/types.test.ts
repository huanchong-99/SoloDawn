// src/components/workflow/types.test.ts
import { describe, it, expect } from 'vitest';
import {
  WizardStep,
  WIZARD_STEPS,
  getDefaultWizardConfig,
  wizardConfigToCreateRequest,
} from './types';

describe('Workflow Types', () => {
  describe('WizardStep enum', () => {
    it('should have all 7 steps defined', () => {
      expect(WizardStep.Project).toBe(0);
      expect(WizardStep.Basic).toBe(1);
      expect(WizardStep.Tasks).toBe(2);
      expect(WizardStep.Models).toBe(3);
      expect(WizardStep.Terminals).toBe(4);
      expect(WizardStep.Commands).toBe(5);
      expect(WizardStep.Advanced).toBe(6);
    });
  });

  describe('WIZARD_STEPS metadata', () => {
    it('should have 7 steps with correct metadata', () => {
      expect(WIZARD_STEPS).toHaveLength(7);
      expect(WIZARD_STEPS[0]).toEqual({
        step: WizardStep.Project,
        nameKey: 'steps.project.name',
        descriptionKey: 'steps.project.description',
      });
    });
  });

  describe('getDefaultWizardConfig', () => {
    it('should return config with all required fields', () => {
      const config = getDefaultWizardConfig();

      expect(config).toHaveProperty('project');
      expect(config).toHaveProperty('basic');
      expect(config).toHaveProperty('tasks');
      expect(config).toHaveProperty('models');
      expect(config).toHaveProperty('terminals');
      expect(config).toHaveProperty('commands');
      expect(config).toHaveProperty('advanced');
    });

    it('should set default basic config with 1 task', () => {
      const config = getDefaultWizardConfig();
      expect(config.basic.taskCount).toBe(1);
    });

    it('should set default target branch to main', () => {
      const config = getDefaultWizardConfig();
      expect(config.advanced.targetBranch).toBe('main');
    });
  });

  describe('wizardConfigToCreateRequest', () => {
    it('should transform minimal config correctly', () => {
      const config = getDefaultWizardConfig();
      config.basic.name = 'Test Workflow';
      config.basic.description = 'Test description';
      config.basic.taskCount = 1;
      config.project.workingDirectory = 'proj-1';

      config.tasks = [
        {
          id: 'task-0',
          name: 'Task 1',
          description: 'First task',
          branch: 'feat/task-1',
          terminalCount: 1,
        },
      ];

      config.models = [
        {
          id: 'model-1',
          displayName: 'Claude 3.5',
          apiType: 'anthropic',
          baseUrl: 'https://api.anthropic.com',
          apiKey: 'sk-ant-xxx',
          modelId: 'claude-3-5-sonnet-20241022',
          isVerified: true,
        },
      ];

      config.terminals = [
        {
          id: 'term-1',
          taskId: 'task-0',
          orderIndex: 0,
          cliTypeId: 'claude-code',
          modelConfigId: 'model-1',
        },
      ];

      config.advanced.orchestrator.modelConfigId = 'model-1';
      config.advanced.mergeTerminal.cliTypeId = 'claude-code';
      config.advanced.mergeTerminal.modelConfigId = 'model-1';

      const request = wizardConfigToCreateRequest('proj-1', config);

      expect(request).toMatchObject({
        projectId: 'proj-1',
        name: 'Test Workflow',
        description: 'Test description',
        executionMode: 'diy',
        useSlashCommands: false,
        commands: [],
        orchestratorConfig: {
          apiType: 'anthropic',
          baseUrl: 'https://api.anthropic.com',
          apiKey: 'sk-ant-xxx',
          model: 'claude-3-5-sonnet-20241022',
        },
        errorTerminalConfig: undefined,
        mergeTerminalConfig: {
          cliTypeId: 'claude-code',
          modelConfigId: 'model-1',
          customBaseUrl: 'https://api.anthropic.com',
          customApiKey: 'sk-ant-xxx',
          modelConfig: {
            displayName: 'Claude 3.5',
            modelId: 'claude-3-5-sonnet-20241022',
          },
        },
        targetBranch: 'main',
        gitWatcherEnabled: true,
      });

      expect(request.tasks).toHaveLength(1);
      expect(request.tasks[0]).toMatchObject({
        id: 'task-0',
        name: 'Task 1',
        description: 'First task',
        branch: 'feat/task-1',
        orderIndex: 0,
      });
      expect(request.tasks[0].terminals).toHaveLength(1);
      expect(request.tasks[0].terminals[0]).toMatchObject({
        id: 'term-1',
        cliTypeId: 'claude-code',
        modelConfigId: 'model-1',
        customBaseUrl: 'https://api.anthropic.com',
        customApiKey: 'sk-ant-xxx',
        modelConfig: {
          displayName: 'Claude 3.5',
          modelId: 'claude-3-5-sonnet-20241022',
        },
        autoConfirm: true,
        orderIndex: 0,
      });
    });

    it('should throw error if orchestrator model not found', () => {
      const config = getDefaultWizardConfig();
      config.basic.name = 'Test';
      config.advanced.orchestrator.modelConfigId = 'non-existent';

      expect(() => wizardConfigToCreateRequest('proj-1', config)).toThrow(
        'Orchestrator model not found'
      );
    });

    it('should throw error if task has no terminals', () => {
      const config = getDefaultWizardConfig();
      config.basic.name = 'Test';
      config.basic.taskCount = 1;
      config.tasks = [{ id: 'task-0', name: 'Task', description: 'Desc', branch: 'feat', terminalCount: 1 }];
      config.models = [
        {
          id: 'model-1',
          displayName: 'Model',
          apiType: 'anthropic',
          baseUrl: 'https://api.test.com',
          apiKey: 'key',
          modelId: 'model',
          isVerified: true,
        },
      ];
      config.terminals = []; // No terminals
      config.advanced.orchestrator.modelConfigId = 'model-1';
      config.advanced.mergeTerminal.cliTypeId = 'claude-code';
      config.advanced.mergeTerminal.modelConfigId = 'model-1';

      expect(() => wizardConfigToCreateRequest('proj-1', config)).toThrow(
        'Task "Task" terminals mismatch: expected 1, got 0'
      );
    });
  });
});
