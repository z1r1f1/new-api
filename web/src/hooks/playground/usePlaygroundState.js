/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useState, useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  getDefaultMessages,
  DEFAULT_CONFIG,
  DEBUG_TABS,
  MESSAGE_STATUS,
} from '../../constants/playground.constants';
import {
  loadConfig,
  saveConfig,
  loadMessages,
  saveMessages,
  loadSessions,
  saveSessions,
  loadSessionState,
  saveActiveSessionId,
  createPlaygroundSession,
  updateSessionMetadata,
} from '../../components/playground/configStorage';
import { processIncompleteThinkTags } from '../../helpers';

export const usePlaygroundState = () => {
  const { t } = useTranslation();

  // 使用惰性初始化，确保只在组件首次挂载时加载配置和消息
  const [savedConfig] = useState(() => loadConfig());
  const [initialSessionState] = useState(() => loadSessionState());
  const [initialMessages] = useState(() => {
    const loaded = initialSessionState.messages || loadMessages();
    // 检查是否是旧的中文默认消息，如果是则清除
    if (
      loaded &&
      loaded.length === 2 &&
      loaded[0].id === '2' &&
      loaded[1].id === '3'
    ) {
      const hasOldChinese =
        loaded[0].content === '你好' ||
        loaded[1].content === '你好，请问有什么可以帮助您的吗？' ||
        loaded[1].content === '你好！很高兴见到你。有什么我可以帮助你的吗？';

      if (hasOldChinese) {
        // 清除旧的默认消息
        localStorage.removeItem('playground_messages');
        return null;
      }
    }
    return loaded;
  });

  // 基础配置状态
  const [inputs, setInputs] = useState(
    savedConfig.inputs || DEFAULT_CONFIG.inputs,
  );
  const [parameterEnabled, setParameterEnabled] = useState(
    savedConfig.parameterEnabled || DEFAULT_CONFIG.parameterEnabled,
  );
  const [showDebugPanel, setShowDebugPanel] = useState(
    savedConfig.showDebugPanel || DEFAULT_CONFIG.showDebugPanel,
  );
  const [customRequestMode, setCustomRequestMode] = useState(
    savedConfig.customRequestMode || DEFAULT_CONFIG.customRequestMode,
  );
  const [customRequestBody, setCustomRequestBody] = useState(
    savedConfig.customRequestBody || DEFAULT_CONFIG.customRequestBody,
  );

  // UI状态
  const [showSettings, setShowSettings] = useState(false);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [status, setStatus] = useState({});
  const [sessions, setSessions] = useState(
    () => initialSessionState.sessions || [],
  );
  const [activeSessionId, setActiveSessionId] = useState(
    () => initialSessionState.activeSessionId || null,
  );

  // 消息相关状态 - 使用加载的消息或默认消息初始化
  const [message, setMessage] = useState(
    () => initialMessages || getDefaultMessages(t),
  );

  // 当语言改变时，如果是默认消息则更新
  useEffect(() => {
    // 只在没有保存的消息时才更新默认消息
    if (!initialMessages) {
      setMessage(getDefaultMessages(t));
    }
  }, [t, initialMessages]); // 当语言改变时

  // 调试状态
  const [debugData, setDebugData] = useState({
    request: null,
    response: null,
    timestamp: null,
    previewRequest: null,
    previewTimestamp: null,
  });
  const [activeDebugTab, setActiveDebugTab] = useState(DEBUG_TABS.PREVIEW);
  const [previewPayload, setPreviewPayload] = useState(null);

  // 编辑状态
  const [editingMessageId, setEditingMessageId] = useState(null);
  const [editValue, setEditValue] = useState('');

  // Refs
  const sseSourceRef = useRef(null);
  const chatRef = useRef(null);
  const saveConfigTimeoutRef = useRef(null);
  const latestMessagesRef = useRef(message);
  const latestActiveSessionIdRef = useRef(activeSessionId);

  useEffect(() => {
    latestMessagesRef.current = message;
  }, [message]);

  useEffect(() => {
    latestActiveSessionIdRef.current = activeSessionId;
  }, [activeSessionId]);

  // 配置更新函数
  const handleInputChange = useCallback((name, value) => {
    setInputs((prev) => ({ ...prev, [name]: value }));
  }, []);

  const handleParameterToggle = useCallback((paramName) => {
    setParameterEnabled((prev) => ({
      ...prev,
      [paramName]: !prev[paramName],
    }));
  }, []);

  // 消息保存函数 - 改为立即保存，可以接受参数
  const saveMessagesImmediately = useCallback(
    (messagesToSave) => {
      // 如果提供了参数，使用参数；否则使用当前状态
      saveMessages(messagesToSave || message, activeSessionId);
      setSessions(loadSessions());
    },
    [message, activeSessionId],
  );

  const switchPlaygroundSession = useCallback((sessionId) => {
    const latestSessions = loadSessions();
    const targetSession = latestSessions.find(
      (session) => session.id === sessionId,
    );
    if (!targetSession) return;

    saveActiveSessionId(targetSession.id);
    setActiveSessionId(targetSession.id);
    setSessions(latestSessions);
    setMessage(targetSession.messages || []);
  }, []);

  const createNewPlaygroundSession = useCallback(() => {
    if (activeSessionId) {
      saveMessages(message, activeSessionId);
    }

    const newSession = createPlaygroundSession([]);
    const nextSessions = [
      newSession,
      ...loadSessions().filter((session) => session.id !== newSession.id),
    ];
    saveSessions(nextSessions);
    saveActiveSessionId(newSession.id);

    setSessions(loadSessions());
    setActiveSessionId(newSession.id);
    setMessage([]);
    return newSession;
  }, [activeSessionId, message]);

  const activeSession =
    sessions.find((session) => session.id === activeSessionId) || null;

  const updateActiveSessionMetadata = useCallback(
    (metadata = {}) => {
      if (!activeSessionId || !metadata || Object.keys(metadata).length === 0) {
        return;
      }
      updateSessionMetadata(activeSessionId, metadata);
      setSessions(loadSessions());
    },
    [activeSessionId],
  );

  // 配置保存
  const debouncedSaveConfig = useCallback(() => {
    if (saveConfigTimeoutRef.current) {
      clearTimeout(saveConfigTimeoutRef.current);
    }

    saveConfigTimeoutRef.current = setTimeout(() => {
      const configToSave = {
        inputs,
        parameterEnabled,
        showDebugPanel,
        customRequestMode,
        customRequestBody,
      };
      saveConfig(configToSave);
    }, 1000);
  }, [
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
  ]);

  // 配置导入/重置
  const handleConfigImport = useCallback((importedConfig) => {
    if (importedConfig.inputs) {
      const parsedMaxTokens = parseInt(importedConfig.inputs.max_tokens, 10);
      setInputs((prev) => ({
        ...prev,
        ...importedConfig.inputs,
        max_tokens: Number.isNaN(parsedMaxTokens)
          ? importedConfig.inputs.max_tokens
          : parsedMaxTokens,
      }));
    }
    if (importedConfig.parameterEnabled) {
      setParameterEnabled((prev) => ({
        ...prev,
        ...importedConfig.parameterEnabled,
      }));
    }
    if (typeof importedConfig.showDebugPanel === 'boolean') {
      setShowDebugPanel(importedConfig.showDebugPanel);
    }
    if (importedConfig.customRequestMode) {
      setCustomRequestMode(importedConfig.customRequestMode);
    }
    if (importedConfig.customRequestBody) {
      setCustomRequestBody(importedConfig.customRequestBody);
    }
    // 如果导入的配置包含消息，也恢复消息
    if (importedConfig.messages && Array.isArray(importedConfig.messages)) {
      setMessage(importedConfig.messages);
    }
    const latestSessionState = loadSessionState();
    setSessions(latestSessionState.sessions || []);
    setActiveSessionId(latestSessionState.activeSessionId || null);
  }, []);

  const handleConfigReset = useCallback(
    (options = {}) => {
      const { resetMessages = false } = options;

      setInputs(DEFAULT_CONFIG.inputs);
      setParameterEnabled(DEFAULT_CONFIG.parameterEnabled);
      setShowDebugPanel(DEFAULT_CONFIG.showDebugPanel);
      setCustomRequestMode(DEFAULT_CONFIG.customRequestMode);
      setCustomRequestBody(DEFAULT_CONFIG.customRequestBody);

      // 只有在明确指定时才重置消息
      if (resetMessages) {
        setMessage([]);
        saveMessagesImmediately([]);
        setTimeout(() => {
          const defaultMessages = getDefaultMessages(t);
          setMessage(defaultMessages);
          saveMessagesImmediately(defaultMessages);
        }, 0);
      }
    },
    [saveMessagesImmediately, t],
  );

  // 清理定时器
  useEffect(() => {
    return () => {
      if (saveConfigTimeoutRef.current) {
        clearTimeout(saveConfigTimeoutRef.current);
      }
    };
  }, []);

  // 离开操练场页面或刷新页面时同步一次最新消息，避免路由切换时丢失会话。
  useEffect(() => {
    const persistLatestMessages = () => {
      saveMessages(latestMessagesRef.current, latestActiveSessionIdRef.current);
    };

    window.addEventListener('pagehide', persistLatestMessages);
    return () => {
      window.removeEventListener('pagehide', persistLatestMessages);
      persistLatestMessages();
    };
  }, []);

  // 页面首次加载时，若最后一条消息仍处于 LOADING/INCOMPLETE 状态，自动修复
  useEffect(() => {
    if (!Array.isArray(message) || message.length === 0) return;

    const lastMsg = message[message.length - 1];
    if (
      lastMsg.status === MESSAGE_STATUS.LOADING ||
      lastMsg.status === MESSAGE_STATUS.INCOMPLETE
    ) {
      if (lastMsg.imageGenerationTask?.id) {
        return;
      }

      const processed = processIncompleteThinkTags(
        lastMsg.content || '',
        lastMsg.reasoningContent || '',
      );

      const fixedLastMsg = {
        ...lastMsg,
        status: MESSAGE_STATUS.COMPLETE,
        content: processed.content,
        reasoningContent: processed.reasoningContent || null,
        isThinkingComplete: true,
      };

      const updatedMessages = [...message.slice(0, -1), fixedLastMsg];
      setMessage(updatedMessages);

      // 保存修复后的消息列表
      setTimeout(() => saveMessagesImmediately(updatedMessages), 0);
    }
  }, []);

  return {
    // 配置状态
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,

    // UI状态
    showSettings,
    models,
    groups,
    status,
    sessions,
    setSessions,
    activeSessionId,
    activeSession,

    // 消息状态
    message,

    // 调试状态
    debugData,
    activeDebugTab,
    previewPayload,

    // 编辑状态
    editingMessageId,
    editValue,

    // Refs
    sseSourceRef,
    chatRef,
    saveConfigTimeoutRef,

    // 更新函数
    setInputs,
    setParameterEnabled,
    setShowDebugPanel,
    setCustomRequestMode,
    setCustomRequestBody,
    setShowSettings,
    setModels,
    setGroups,
    setStatus,
    setMessage,
    setDebugData,
    setActiveDebugTab,
    setPreviewPayload,
    setEditingMessageId,
    setEditValue,

    // 处理函数
    handleInputChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    switchPlaygroundSession,
    createNewPlaygroundSession,
    updateActiveSessionMetadata,
    handleConfigImport,
    handleConfigReset,
  };
};
