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
  getUserIdFromLocalStorage,
  showError,
  formatMessageForAPI,
  isValidMessage,
  getTextContent,
} from './utils';
import axios from 'axios';
import { MESSAGE_ROLES, MESSAGE_STATUS } from '../constants/playground.constants';

export let API = axios.create({
  baseURL: import.meta.env.VITE_REACT_APP_SERVER_URL
    ? import.meta.env.VITE_REACT_APP_SERVER_URL
    : '',
  headers: {
    'New-API-User': getUserIdFromLocalStorage(),
    'Cache-Control': 'no-store',
  },
});

function redirectToOAuthUrl(url, options = {}) {
  const { openInNewTab = false } = options;
  const targetUrl = typeof url === 'string' ? url : url.toString();

  if (openInNewTab) {
    window.open(targetUrl, '_blank');
    return;
  }

  window.location.assign(targetUrl);
}

function patchAPIInstance(instance) {
  const originalGet = instance.get.bind(instance);
  const inFlightGetRequests = new Map();

  const genKey = (url, config = {}) => {
    const params = config.params ? JSON.stringify(config.params) : '{}';
    return `${url}?${params}`;
  };

  instance.get = (url, config = {}) => {
    if (config?.disableDuplicate) {
      return originalGet(url, config);
    }

    const key = genKey(url, config);
    if (inFlightGetRequests.has(key)) {
      return inFlightGetRequests.get(key);
    }

    const reqPromise = originalGet(url, config).finally(() => {
      inFlightGetRequests.delete(key);
    });

    inFlightGetRequests.set(key, reqPromise);
    return reqPromise;
  };
}

patchAPIInstance(API);

export function updateAPI() {
  API = axios.create({
    baseURL: import.meta.env.VITE_REACT_APP_SERVER_URL
      ? import.meta.env.VITE_REACT_APP_SERVER_URL
      : '',
    headers: {
      'New-API-User': getUserIdFromLocalStorage(),
      'Cache-Control': 'no-store',
    },
  });

  patchAPIInstance(API);
}

API.interceptors.response.use(
  (response) => response,
  (error) => {
    // 如果请求配置中显式要求跳过全局错误处理，则不弹出默认错误提示
    if (error.config && error.config.skipErrorHandler) {
      return Promise.reject(error);
    }
    showError(error);
    return Promise.reject(error);
  },
);

// playground

export const isImageGenerationModel = (model) => {
  const normalized = String(model || '')
    .trim()
    .toLowerCase();
  return (
    normalized.startsWith('gpt-image') ||
    normalized.startsWith('chatgpt-image') ||
    normalized.startsWith('dall-e')
  );
};

const extractImageUrlsFromContent = (content) => {
  if (!Array.isArray(content)) {
    if (typeof content !== 'string') {
      return [];
    }
    const markdownImageUrls = [];
    const imageRegex = /!\[[^\]]*]\(([^)]+)\)/g;
    let match;
    while ((match = imageRegex.exec(content)) !== null) {
      if (match[1]) {
        markdownImageUrls.push(match[1]);
      }
    }
    return markdownImageUrls;
  }
  return content
    .filter((item) => item?.type === 'image_url')
    .map((item) => item?.image_url?.url || item?.image_url)
    .filter((url) => typeof url === 'string' && url.trim() !== '')
    .map((url) => url.trim());
};

const normalizeImageReferenceUrl = (url) => {
  const trimmed = String(url || '').trim();
  if (!trimmed) return '';
  if (
    trimmed.startsWith('data:') ||
    trimmed.startsWith('http://') ||
    trimmed.startsWith('https://')
  ) {
    return trimmed;
  }
  if (trimmed.startsWith('/') && typeof window !== 'undefined') {
    return `${window.location.origin}${trimmed}`;
  }
  return trimmed;
};

const dedupeStrings = (items) => {
  const seen = new Set();
  return items.filter((item) => {
    const value = String(item || '').trim();
    if (!value || seen.has(value)) return false;
    seen.add(value);
    return true;
  });
};

const MAX_FALLBACK_REFERENCE_IMAGES = 4;
const MAX_FALLBACK_REFERENCE_BYTES = 6 * 1024 * 1024;

const isDataUrl = (url) =>
  String(url || '')
    .trim()
    .startsWith('data:');

const estimateStringBytes = (value) => {
  const text = String(value || '');
  if (typeof TextEncoder !== 'undefined') {
    return new TextEncoder().encode(text).length;
  }
  return text.length;
};

const describeImageForPrompt = (url) => {
  const trimmed = String(url || '').trim();
  if (!trimmed) return '';
  if (!isDataUrl(trimmed)) {
    return trimmed;
  }
  const mime = trimmed.slice(5, trimmed.indexOf(';')).trim() || 'image';
  return `[图片 data URL 已省略，mime=${mime}，大小约 ${Math.ceil(estimateStringBytes(trimmed) / 1024)}KB]`;
};

const limitFallbackReferenceImages = (urls) => {
  const out = [];
  let totalBytes = 0;
  dedupeStrings(urls)
    .slice(-MAX_FALLBACK_REFERENCE_IMAGES * 2)
    .reverse()
    .some((url) => {
      const bytes = estimateStringBytes(url);
      if (out.length >= MAX_FALLBACK_REFERENCE_IMAGES) {
        return true;
      }
      if (isDataUrl(url) && totalBytes + bytes > MAX_FALLBACK_REFERENCE_BYTES) {
        return false;
      }
      out.push(url);
      totalBytes += bytes;
      return false;
    });
  return out.reverse();
};

const buildImageConversationContext = (messages, lastUserIndex) => {
  const contextMessages = messages
    .slice(Math.max(0, lastUserIndex - 8), lastUserIndex)
    .filter(shouldIncludeInConversationContext);
  const parts = contextMessages
    .map((msg) => {
      const text = getTextContent(msg).trim();
      const images = extractImageUrlsFromContent(msg.content).map(
        describeImageForPrompt,
      );
      const roleLabel =
        msg.role === MESSAGE_ROLES.USER
          ? '用户'
          : msg.role === MESSAGE_ROLES.ASSISTANT
            ? '助手'
            : '系统';
      const imageNote =
        images.length > 0 ? `\n[该消息包含 ${images.length} 张图片]` : '';
      const content = [text, imageNote].filter(Boolean).join('');
      return content ? `${roleLabel}: ${content}` : '';
    })
    .filter(Boolean);

  const context = parts.join('\n');
  return context.length > 4000 ? context.slice(-4000) : context;
};

const FALLBACK_PROMPT_ERROR_PREFIXES = [
  '请求发生错误',
  'An error occurred with the request',
  'HTTP error!',
  'Panic detected',
  'chatgpt upstream 401:',
  'chatgpt image channel:',
];

const isFallbackPromptErrorMessage = (message) => {
  if (!message) {
    return false;
  }

  if (message.status === MESSAGE_STATUS.ERROR) {
    return true;
  }

  const text = getTextContent(message).trim();
  if (!text) {
    return false;
  }

  return FALLBACK_PROMPT_ERROR_PREFIXES.some((prefix) =>
    text.startsWith(prefix),
  );
};

const shouldIncludeInConversationContext = (message) =>
  isValidMessage(message) && !isFallbackPromptErrorMessage(message);

const buildChatGPTWebFallbackPrompt = (messages, systemPrompt = '') => {
  const parts = [];
  if (systemPrompt && systemPrompt.trim()) {
    parts.push(`System: ${systemPrompt.trim()}`);
  }
  messages.filter(shouldIncludeInConversationContext).forEach((msg) => {
    const text = getTextContent(msg).trim();
    const images = extractImageUrlsFromContent(msg.content)
      .map(normalizeImageReferenceUrl)
      .map(describeImageForPrompt)
      .filter(Boolean);
    if (!text && images.length === 0) {
      return;
    }
    const roleLabel =
      msg.role === MESSAGE_ROLES.USER
        ? 'User'
        : msg.role === MESSAGE_ROLES.ASSISTANT
          ? 'Assistant'
          : msg.role === MESSAGE_ROLES.SYSTEM
            ? 'System'
            : msg.role || 'Message';
    const imageText =
      images.length > 0 ? `\nImages:\n${images.join('\n')}` : '';
    parts.push(`${roleLabel}: ${[text, imageText].filter(Boolean).join('')}`);
  });
  return parts.join('\n\n').trim();
};

const buildImageGenerationPayload = (
  messages,
  systemPrompt,
  inputs,
  sessionContext = {},
) => {
  const lastUserIndex = (() => {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      if (messages[i].role === MESSAGE_ROLES.USER) return i;
    }
    return -1;
  })();
  const lastUserMessage = lastUserIndex >= 0 ? messages[lastUserIndex] : null;
  const webConversationId = String(
    sessionContext.webConversationId || '',
  ).trim();
  const conversationContext = webConversationId
    ? ''
    : buildImageConversationContext(
        messages,
        lastUserIndex >= 0 ? lastUserIndex : messages.length,
      );
  const fallbackPrompt = webConversationId
    ? buildChatGPTWebFallbackPrompt(messages, systemPrompt)
    : '';
  const promptParts = [];
  if (systemPrompt && systemPrompt.trim()) {
    promptParts.push(systemPrompt.trim());
  }
  if (conversationContext) {
    promptParts.push(
      '以下是当前操练场会话上文，请在生成图片时保持这些上下文：',
    );
    promptParts.push(conversationContext);
    promptParts.push('用户最新要求：');
  }
  promptParts.push(getTextContent(lastUserMessage) || '生成一张图片');

  const imageUrls = dedupeStrings(
    extractImageUrlsFromContent(lastUserMessage?.content).map(
      normalizeImageReferenceUrl,
    ),
  );
  const priorImageUrls = webConversationId
    ? []
    : dedupeStrings(
        messages
          .slice(0, lastUserIndex >= 0 ? lastUserIndex : messages.length)
          .flatMap((msg) => extractImageUrlsFromContent(msg.content))
          .map(normalizeImageReferenceUrl),
      );
  const fallbackReferenceImageUrls = webConversationId
    ? limitFallbackReferenceImages(
        messages
          .slice(0, lastUserIndex >= 0 ? lastUserIndex : messages.length)
          .flatMap((msg) => extractImageUrlsFromContent(msg.content))
          .map(normalizeImageReferenceUrl),
      )
    : [];
  const payload = {
    model: inputs.model,
    group: inputs.group,
    prompt: promptParts.filter(Boolean).join('\n\n'),
  };
  if (webConversationId) {
    payload.conversation_id = webConversationId;
    if (fallbackPrompt) {
      payload.fallback_prompt = fallbackPrompt;
    }
    if (fallbackReferenceImageUrls.length > 0) {
      payload.fallback_reference_images = fallbackReferenceImageUrls;
    }
  }
  if (imageUrls.length === 1) {
    payload.image = imageUrls[0];
  } else if (imageUrls.length > 1) {
    payload.image = imageUrls;
  }
  if (priorImageUrls.length > 0) {
    payload.reference_images = priorImageUrls;
  }
  return payload;
};

// 构建API请求负载
export const buildApiPayload = (
  messages,
  systemPrompt,
  inputs,
  parameterEnabled,
  sessionContext = {},
) => {
  let processedMessages = messages
    .filter(isValidMessage)
    .map(formatMessageForAPI)
    .filter(Boolean);

  if (isImageGenerationModel(inputs.model)) {
    return buildImageGenerationPayload(
      processedMessages,
      systemPrompt,
      inputs,
      sessionContext,
    );
  }

  const webConversationId = String(
    sessionContext.webConversationId || '',
  ).trim();
  const fallbackPrompt = webConversationId
    ? buildChatGPTWebFallbackPrompt(processedMessages, systemPrompt)
    : '';
  if (webConversationId) {
    const lastUserMessage = [...processedMessages]
      .reverse()
      .find((msg) => msg.role === MESSAGE_ROLES.USER);
    processedMessages = lastUserMessage ? [lastUserMessage] : [];
  }

  // 如果有系统提示，插入到消息开头
  if (systemPrompt && systemPrompt.trim()) {
    processedMessages.unshift({
      role: MESSAGE_ROLES.SYSTEM,
      content: systemPrompt.trim(),
    });
  }

  const payload = {
    model: inputs.model,
    group: inputs.group,
    messages: processedMessages,
    stream: inputs.stream,
  };
  if (webConversationId) {
    payload.conversation_id = webConversationId;
    if (fallbackPrompt) {
      payload.fallback_prompt = fallbackPrompt;
    }
  }

  // 添加启用的参数
  const parameterMappings = {
    temperature: 'temperature',
    top_p: 'top_p',
    max_tokens: 'max_tokens',
    frequency_penalty: 'frequency_penalty',
    presence_penalty: 'presence_penalty',
    seed: 'seed',
  };

  Object.entries(parameterMappings).forEach(([key, param]) => {
    const enabled = parameterEnabled[key];
    const value = inputs[param];
    const hasValue = value !== undefined && value !== null;

    if (!enabled) {
      return;
    }

    if (param === 'max_tokens') {
      if (typeof value === 'number') {
        payload[param] = value;
      }
      return;
    }

    if (hasValue) {
      payload[param] = value;
    }
  });

  return payload;
};

export const normalizePlaygroundPayloadForTransport = (payload) => {
  if (!payload || typeof payload !== 'object') {
    return payload;
  }

  if (!isImageGenerationModel(payload.model)) {
    return payload;
  }

  if (!Array.isArray(payload.messages) || payload.messages.length === 0) {
    return payload;
  }

  const normalizedImagePayload = buildImageGenerationPayload(
    payload.messages,
    '',
    {
      model: payload.model,
      group: payload.group,
    },
    {
      webConversationId: String(payload.conversation_id || '').trim(),
    },
  );

  const mergedPayload = {
    ...normalizedImagePayload,
    ...payload,
  };

  delete mergedPayload.messages;
  delete mergedPayload.stream;

  return mergedPayload;
};

// 处理API错误响应
export const handleApiError = (error, response = null) => {
  const errorInfo = {
    error: error.message || '未知错误',
    timestamp: new Date().toISOString(),
    stack: error.stack,
  };

  if (response) {
    errorInfo.status = response.status;
    errorInfo.statusText = response.statusText;
  }

  if (error.message.includes('HTTP error')) {
    errorInfo.details = '服务器返回了错误状态码';
  } else if (error.message.includes('Failed to fetch')) {
    errorInfo.details = '网络连接失败或服务器无响应';
  }

  return errorInfo;
};

// 处理模型数据
export const processModelsData = (data, currentModel) => {
  const modelOptions = data.map((model) => ({
    label: model,
    value: model,
  }));

  const hasCurrentModel = modelOptions.some(
    (option) => option.value === currentModel,
  );
  const selectedModel =
    hasCurrentModel && modelOptions.length > 0
      ? currentModel
      : modelOptions[0]?.value;

  return { modelOptions, selectedModel };
};

// 处理分组数据
export const processGroupsData = (data, userGroup, currentGroup = '') => {
  let groupOptions = Object.entries(data).map(([group, info]) => {
    const description = String(info?.desc || '').trim();
    const labelSource = description || group;
    return {
      label:
        labelSource.length > 20
          ? labelSource.substring(0, 20) + '...'
          : labelSource,
      value: group,
      ratio: info.ratio,
      fullLabel: description || group,
    };
  });

  const selectedGroup = String(currentGroup || '').trim();
  if (selectedGroup && !groupOptions.some((g) => g.value === selectedGroup)) {
    groupOptions.unshift({
      label: selectedGroup,
      value: selectedGroup,
      ratio: 1,
      fullLabel: selectedGroup,
    });
  }

  if (groupOptions.length === 0) {
    groupOptions = [
      {
        label: '用户分组',
        value: '',
        ratio: 1,
      },
    ];
  } else if (userGroup) {
    const userGroupIndex = groupOptions.findIndex((g) => g.value === userGroup);
    if (userGroupIndex > -1) {
      const userGroupOption = groupOptions.splice(userGroupIndex, 1)[0];
      groupOptions.unshift(userGroupOption);
    }
  }

  return groupOptions;
};

// 原来components中的utils.js

export async function getOAuthState() {
  let path = '/api/oauth/state';
  let affCode = localStorage.getItem('aff');
  if (affCode && affCode.length > 0) {
    path += `?aff=${affCode}`;
  }
  const res = await API.get(path);
  const { success, message, data } = res.data;
  if (success) {
    return data;
  } else {
    showError(message);
    return '';
  }
}

async function prepareOAuthState(options = {}) {
  const { shouldLogout = false } = options;
  if (shouldLogout) {
    try {
      await API.get('/api/user/logout', { skipErrorHandler: true });
    } catch (err) {}
    localStorage.removeItem('user');
    updateAPI();
  }
  return await getOAuthState();
}

export async function onDiscordOAuthClicked(client_id, options = {}) {
  const state = await prepareOAuthState(options);
  if (!state) return;
  const redirect_uri = `${window.location.origin}/oauth/discord`;
  const response_type = 'code';
  const scope = 'identify+openid';
  redirectToOAuthUrl(
    `https://discord.com/oauth2/authorize?client_id=${client_id}&redirect_uri=${redirect_uri}&response_type=${response_type}&scope=${scope}&state=${state}`,
  );
}

export async function onOIDCClicked(
  auth_url,
  client_id,
  openInNewTab = false,
  options = {},
) {
  const state = await prepareOAuthState(options);
  if (!state) return;
  const url = new URL(auth_url);
  url.searchParams.set('client_id', client_id);
  url.searchParams.set('redirect_uri', `${window.location.origin}/oauth/oidc`);
  url.searchParams.set('response_type', 'code');
  url.searchParams.set('scope', 'openid profile email');
  url.searchParams.set('state', state);
  redirectToOAuthUrl(url, { openInNewTab });
}

export async function onGitHubOAuthClicked(github_client_id, options = {}) {
  const state = await prepareOAuthState(options);
  if (!state) return;
  redirectToOAuthUrl(
    `https://github.com/login/oauth/authorize?client_id=${github_client_id}&state=${state}&scope=user:email`,
  );
}

export async function onLinuxDOOAuthClicked(
  linuxdo_client_id,
  options = { shouldLogout: false },
) {
  const state = await prepareOAuthState(options);
  if (!state) return;
  redirectToOAuthUrl(
    `https://connect.linux.do/oauth2/authorize?response_type=code&client_id=${linuxdo_client_id}&state=${state}`,
  );
}

/**
 * Initiate custom OAuth login
 * @param {Object} provider - Custom OAuth provider config from status API
 * @param {string} provider.slug - Provider slug (used for callback URL)
 * @param {string} provider.client_id - OAuth client ID
 * @param {string} provider.authorization_endpoint - Authorization URL
 * @param {string} provider.scopes - OAuth scopes (space-separated)
 * @param {Object} options - Options
 * @param {boolean} options.shouldLogout - Whether to logout first
 */
export async function onCustomOAuthClicked(provider, options = {}) {
  const state = await prepareOAuthState(options);
  if (!state) return;

  try {
    const redirect_uri = `${window.location.origin}/oauth/${provider.slug}`;

    // Check if authorization_endpoint is a full URL or relative path
    let authUrl;
    if (
      provider.authorization_endpoint.startsWith('http://') ||
      provider.authorization_endpoint.startsWith('https://')
    ) {
      authUrl = new URL(provider.authorization_endpoint);
    } else {
      // Relative path - this is a configuration error, show error message
      console.error(
        'Custom OAuth authorization_endpoint must be a full URL:',
        provider.authorization_endpoint,
      );
      showError(
        'OAuth 配置错误：授权端点必须是完整的 URL（以 http:// 或 https:// 开头）',
      );
      return;
    }

    authUrl.searchParams.set('client_id', provider.client_id);
    authUrl.searchParams.set('redirect_uri', redirect_uri);
    authUrl.searchParams.set('response_type', 'code');
    authUrl.searchParams.set(
      'scope',
      provider.scopes || 'openid profile email',
    );
    authUrl.searchParams.set('state', state);

    redirectToOAuthUrl(authUrl);
  } catch (error) {
    console.error('Failed to initiate custom OAuth:', error);
    showError('OAuth 登录失败：' + (error.message || '未知错误'));
  }
}

let channelModels = undefined;
export async function loadChannelModels() {
  const res = await API.get('/api/models');
  const { success, data } = res.data;
  if (!success) {
    return;
  }
  channelModels = data;
  localStorage.setItem('channel_models', JSON.stringify(data));
}

export function getChannelModels(type) {
  if (channelModels !== undefined && type in channelModels) {
    if (!channelModels[type]) {
      return [];
    }
    return channelModels[type];
  }
  let models = localStorage.getItem('channel_models');
  if (!models) {
    return [];
  }
  channelModels = JSON.parse(models);
  if (type in channelModels) {
    return channelModels[type];
  }
  return [];
}
