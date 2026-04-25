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

import {
  STORAGE_KEYS,
  DEFAULT_CONFIG,
} from '../../constants/playground.constants';
import { getTextContent } from '../../helpers/utils';

const MAX_SESSION_COUNT = 30;
const MAX_PERSISTED_DATA_URL_BYTES = 256 * 1024;

const generateSessionId = () => {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `pg_${crypto.randomUUID()}`;
  }
  return `pg_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
};

const parseJSON = (value, fallback = null) => {
  if (!value) return fallback;
  try {
    return JSON.parse(value);
  } catch (_) {
    return fallback;
  }
};

const getFirstUserText = (messages = []) => {
  const userMessage = messages.find((item) => item?.role === 'user');
  return getTextContent(userMessage).trim();
};

const buildSessionTitle = (messages = []) => {
  const firstUserText = getFirstUserText(messages);
  if (!firstUserText) return '新会话';
  return firstUserText.length > 24
    ? `${firstUserText.slice(0, 24)}...`
    : firstUserText;
};

const normalizeMessages = (messages) =>
  Array.isArray(messages) ? messages : [];

const estimateStringBytes = (value) => {
  const text = String(value || '');
  if (typeof TextEncoder !== 'undefined') {
    return new TextEncoder().encode(text).length;
  }
  return text.length;
};

const isLargeDataUrl = (value) =>
  typeof value === 'string' &&
  value.startsWith('data:') &&
  estimateStringBytes(value) > MAX_PERSISTED_DATA_URL_BYTES;

const sanitizeContentForStorage = (content) => {
  if (!Array.isArray(content)) {
    return content;
  }

  let omittedImageCount = 0;
  const sanitized = content.filter((item) => {
    if (item?.type !== 'image_url') {
      return true;
    }
    const imageUrl = item?.image_url?.url || item?.image_url;
    if (isLargeDataUrl(imageUrl)) {
      omittedImageCount += 1;
      return false;
    }
    return true;
  });

  if (omittedImageCount > 0) {
    sanitized.push({
      type: 'text',
      text: `[${omittedImageCount} 张本地图片因浏览器存储空间限制未保存，如需继续编辑请重新上传]`,
    });
  }

  return sanitized;
};

const sanitizeMessagesForStorage = (messages) =>
  normalizeMessages(messages).map((message) => ({
    ...message,
    content: sanitizeContentForStorage(message?.content),
  }));

const normalizeSession = (session) => {
  const now = new Date().toISOString();
  const messages = normalizeMessages(session?.messages);
  return {
    id: session?.id || generateSessionId(),
    title: session?.title || buildSessionTitle(messages),
    messages,
    createdAt: session?.createdAt || session?.timestamp || now,
    updatedAt: session?.updatedAt || session?.timestamp || now,
    webConversationId: session?.webConversationId || '',
  };
};

const trimSessions = (sessions) =>
  [...sessions]
    .sort((a, b) => new Date(b.updatedAt || 0) - new Date(a.updatedAt || 0))
    .slice(0, MAX_SESSION_COUNT);

const getStoredSessions = () => {
  const parsed = parseJSON(localStorage.getItem(STORAGE_KEYS.SESSIONS), []);
  return Array.isArray(parsed) ? parsed.map(normalizeSession) : [];
};

const persistSessions = (sessions) => {
  localStorage.setItem(
    STORAGE_KEYS.SESSIONS,
    JSON.stringify(trimSessions(sessions.map(normalizeSession))),
  );
};

const getLegacyMessages = () => {
  const parsed = parseJSON(localStorage.getItem(STORAGE_KEYS.MESSAGES), null);
  return normalizeMessages(parsed?.messages);
};

const ensureSessions = () => {
  const existingSessions = getStoredSessions();
  if (existingSessions.length > 0) return trimSessions(existingSessions);

  const legacyMessages = getLegacyMessages();
  const initialSession = normalizeSession({
    id: generateSessionId(),
    title: buildSessionTitle(legacyMessages),
    messages: legacyMessages,
  });
  persistSessions([initialSession]);
  localStorage.setItem(STORAGE_KEYS.ACTIVE_SESSION_ID, initialSession.id);
  return [initialSession];
};

export const loadSessions = () => ensureSessions();

export const saveSessions = (sessions) => {
  try {
    persistSessions(Array.isArray(sessions) ? sessions : []);
  } catch (error) {
    console.error('保存会话失败:', error);
  }
};

export const loadActiveSessionId = () => {
  try {
    const sessions = ensureSessions();
    const storedActiveId = localStorage.getItem(STORAGE_KEYS.ACTIVE_SESSION_ID);
    const activeSession = sessions.find((item) => item.id === storedActiveId);
    if (activeSession) return activeSession.id;

    const fallbackId = sessions[0]?.id || null;
    if (fallbackId) {
      localStorage.setItem(STORAGE_KEYS.ACTIVE_SESSION_ID, fallbackId);
    }
    return fallbackId;
  } catch (error) {
    console.error('加载当前会话失败:', error);
    return null;
  }
};

export const saveActiveSessionId = (sessionId) => {
  try {
    if (sessionId) {
      localStorage.setItem(STORAGE_KEYS.ACTIVE_SESSION_ID, sessionId);
    }
  } catch (error) {
    console.error('保存当前会话失败:', error);
  }
};

export const createPlaygroundSession = (messages = []) =>
  normalizeSession({
    id: generateSessionId(),
    title: buildSessionTitle(messages),
    messages: normalizeMessages(messages),
  });

export const updateSessionMetadata = (sessionId, metadata = {}) => {
  try {
    if (!sessionId || !metadata || Object.keys(metadata).length === 0) return;
    const sessions = ensureSessions();
    const now = new Date().toISOString();
    const updatedSessions = sessions.map((session) =>
      session.id === sessionId
        ? normalizeSession({
            ...session,
            ...metadata,
            updatedAt: metadata.updatedAt || now,
          })
        : session,
    );
    persistSessions(updatedSessions);
  } catch (error) {
    console.error('更新会话失败:', error);
  }
};

export const loadSessionState = () => {
  const sessions = loadSessions();
  const activeSessionId = loadActiveSessionId();
  const activeSession = sessions.find((item) => item.id === activeSessionId);
  return {
    sessions,
    activeSessionId,
    messages: activeSession?.messages || null,
  };
};

/**
 * 保存配置到 localStorage
 * @param {Object} config - 要保存的配置对象
 */
export const saveConfig = (config) => {
  try {
    const configToSave = {
      ...config,
      timestamp: new Date().toISOString(),
    };
    localStorage.setItem(STORAGE_KEYS.CONFIG, JSON.stringify(configToSave));
  } catch (error) {
    console.error('保存配置失败:', error);
  }
};

/**
 * 保存消息到 localStorage
 * @param {Array} messages - 要保存的消息数组
 * @param {string|null} sessionId - 要更新的会话 ID；为空时更新当前会话
 */
export const saveMessages = (messages, sessionId = null) => {
  const persistMessages = (messagesToPersist) => {
    const normalizedMessages = normalizeMessages(messagesToPersist);
    const messagesToSave = {
      messages: normalizedMessages,
      timestamp: new Date().toISOString(),
    };
    localStorage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messagesToSave));

    const sessions = ensureSessions();
    const activeId = sessionId || loadActiveSessionId() || sessions[0]?.id;
    if (!activeId) return;

    const now = new Date().toISOString();
    let found = false;
    const updatedSessions = sessions.map((session) => {
      if (session.id !== activeId) return session;
      found = true;
      const title =
        session.title && session.title !== '新会话'
          ? session.title
          : buildSessionTitle(normalizedMessages);
      return normalizeSession({
        ...session,
        title,
        messages: normalizedMessages,
        updatedAt: now,
      });
    });

    if (!found) {
      updatedSessions.unshift(
        normalizeSession({
          id: activeId,
          title: buildSessionTitle(normalizedMessages),
          messages: normalizedMessages,
          createdAt: now,
          updatedAt: now,
        }),
      );
    }

    persistSessions(updatedSessions);
  };

  try {
    persistMessages(messages);
  } catch (error) {
    try {
      persistMessages(sanitizeMessagesForStorage(messages));
      console.warn('消息包含过大的本地图片，已省略图片后保存会话:', error);
    } catch (retryError) {
      console.error('保存消息失败:', retryError);
    }
  }
};

/**
 * 从 localStorage 加载配置
 * @returns {Object} 配置对象，如果不存在则返回默认配置
 */
export const loadConfig = () => {
  try {
    const savedConfig = localStorage.getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);
      const parsedMaxTokens = parseInt(parsedConfig?.inputs?.max_tokens, 10);

      const mergedConfig = {
        inputs: {
          ...DEFAULT_CONFIG.inputs,
          ...parsedConfig.inputs,
          max_tokens: Number.isNaN(parsedMaxTokens)
            ? parsedConfig?.inputs?.max_tokens
            : parsedMaxTokens,
        },
        parameterEnabled: {
          ...DEFAULT_CONFIG.parameterEnabled,
          ...parsedConfig.parameterEnabled,
        },
        showDebugPanel:
          parsedConfig.showDebugPanel || DEFAULT_CONFIG.showDebugPanel,
        customRequestMode:
          parsedConfig.customRequestMode || DEFAULT_CONFIG.customRequestMode,
        customRequestBody:
          parsedConfig.customRequestBody || DEFAULT_CONFIG.customRequestBody,
      };

      return mergedConfig;
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }

  return DEFAULT_CONFIG;
};

/**
 * 从 localStorage 加载消息
 * @returns {Array} 消息数组，如果不存在则返回 null
 */
export const loadMessages = () => {
  try {
    const { messages } = loadSessionState();
    return messages || null;
  } catch (error) {
    console.error('加载消息失败:', error);
  }

  return null;
};

/**
 * 清除保存的配置
 */
export const clearConfig = () => {
  try {
    localStorage.removeItem(STORAGE_KEYS.CONFIG);
    localStorage.removeItem(STORAGE_KEYS.MESSAGES); // 同时清除消息
    localStorage.removeItem(STORAGE_KEYS.SESSIONS);
    localStorage.removeItem(STORAGE_KEYS.ACTIVE_SESSION_ID);
  } catch (error) {
    console.error('清除配置失败:', error);
  }
};

/**
 * 清除保存的消息
 */
export const clearMessages = () => {
  try {
    localStorage.removeItem(STORAGE_KEYS.MESSAGES);
    const activeId = loadActiveSessionId();
    if (activeId) {
      saveMessages([], activeId);
    }
  } catch (error) {
    console.error('清除消息失败:', error);
  }
};

/**
 * 检查是否有保存的配置
 * @returns {boolean} 是否存在保存的配置
 */
export const hasStoredConfig = () => {
  try {
    return localStorage.getItem(STORAGE_KEYS.CONFIG) !== null;
  } catch (error) {
    console.error('检查配置失败:', error);
    return false;
  }
};

/**
 * 获取配置的最后保存时间
 * @returns {string|null} 最后保存时间的 ISO 字符串
 */
export const getConfigTimestamp = () => {
  try {
    const savedConfig = localStorage.getItem(STORAGE_KEYS.CONFIG);
    if (savedConfig) {
      const parsedConfig = JSON.parse(savedConfig);
      return parsedConfig.timestamp || null;
    }
  } catch (error) {
    console.error('获取配置时间戳失败:', error);
  }
  return null;
};

/**
 * 导出配置为 JSON 文件（包含消息）
 * @param {Object} config - 要导出的配置
 * @param {Array} messages - 要导出的消息
 */
export const exportConfig = (config, messages = null) => {
  try {
    const configToExport = {
      ...config,
      messages: messages || loadMessages(), // 包含消息数据
      sessions: loadSessions(),
      activeSessionId: loadActiveSessionId(),
      exportTime: new Date().toISOString(),
      version: '1.1',
    };

    const dataStr = JSON.stringify(configToExport, null, 2);
    const dataBlob = new Blob([dataStr], { type: 'application/json' });

    const link = document.createElement('a');
    link.href = URL.createObjectURL(dataBlob);
    link.download = `playground-config-${new Date().toISOString().split('T')[0]}.json`;
    link.click();

    URL.revokeObjectURL(link.href);
  } catch (error) {
    console.error('导出配置失败:', error);
  }
};

/**
 * 从文件导入配置（包含消息）
 * @param {File} file - 包含配置的 JSON 文件
 * @returns {Promise<Object>} 导入的配置对象
 */
export const importConfig = (file) => {
  return new Promise((resolve, reject) => {
    try {
      const reader = new FileReader();
      reader.onload = (e) => {
        try {
          const importedConfig = JSON.parse(e.target.result);

          if (importedConfig.inputs && importedConfig.parameterEnabled) {
            if (
              importedConfig.sessions &&
              Array.isArray(importedConfig.sessions) &&
              importedConfig.sessions.length > 0
            ) {
              const normalizedSessions =
                importedConfig.sessions.map(normalizeSession);
              saveSessions(normalizedSessions);
              saveActiveSessionId(
                importedConfig.activeSessionId || normalizedSessions[0]?.id,
              );
            } else if (
              importedConfig.messages &&
              Array.isArray(importedConfig.messages)
            ) {
              saveMessages(importedConfig.messages);
            }

            resolve(importedConfig);
          } else {
            reject(new Error('配置文件格式无效'));
          }
        } catch (parseError) {
          reject(new Error('解析配置文件失败: ' + parseError.message));
        }
      };
      reader.onerror = () => reject(new Error('读取文件失败'));
      reader.readAsText(file);
    } catch (error) {
      reject(new Error('导入配置失败: ' + error.message));
    }
  });
};
