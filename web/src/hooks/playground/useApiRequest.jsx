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

import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { SSE } from 'sse.js';
import {
  API_ENDPOINTS,
  MESSAGE_STATUS,
  DEBUG_TABS,
} from '../../constants/playground.constants';
import {
  getUserIdFromLocalStorage,
  handleApiError,
  normalizePlaygroundPayloadForTransport,
  processThinkTags,
  processIncompleteThinkTags,
} from '../../helpers';

const isImageGenerationPayload = (payload) =>
  payload &&
  Object.prototype.hasOwnProperty.call(payload, 'prompt') &&
  !payload.messages;

const getEndpointForPayload = (payload) =>
  isImageGenerationPayload(payload)
    ? API_ENDPOINTS.IMAGE_GENERATIONS
    : API_ENDPOINTS.CHAT_COMPLETIONS;

const imageResponseToMarkdown = (data) => {
  const items = Array.isArray(data?.data) ? data.data : [];
  return items
    .map((item, index) => {
      const url =
        item?.url ||
        (item?.b64_json ? `data:image/png;base64,${item.b64_json}` : '');
      if (!url) return '';
      const revised = item?.revised_prompt
        ? `**Revised prompt:** ${item.revised_prompt}\n\n`
        : '';
      return `${revised}![generated image ${index + 1}](${url})`;
    })
    .filter(Boolean)
    .join('\n\n');
};

const markdownImageRegex = /!\[([^\]]*)]\((data:image\/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]+|\/pg\/images\/generations\/[^)\s]+|https?:\/\/[^)\s]+)\)/g;

const hasInlineDataImagePayload = (content) =>
  typeof content === 'string' &&
  (content.includes('data:image/') || content.includes('/pg/images/generations/'));

const stripSkippedMainlineMetadata = (content) =>
  String(content || '')
    .split('\n')
    .filter((line) => line.trim() !== '{"skipped_mainline":true}')
    .join('\n')
    .trim();

const isGeneratedImageAltText = (text) =>
  /^image[_\s-]*\d+$/i.test(String(text || '').trim()) ||
  /^generated image\s+\d+$/i.test(String(text || '').trim());

const markdownImagesToMessageContent = (content) => {
  if (typeof content !== 'string' || !content.trim()) {
    return content;
  }
  const normalized = stripSkippedMainlineMetadata(content);
  const images = [];
  const text = normalized
    .replace(markdownImageRegex, (_match, alt, url) => {
      const safeAlt = String(alt || '').trim();
      images.push({
        type: 'image_url',
        image_url: { url: String(url || '').replace(/[\r\n]/g, '') },
      });
      return safeAlt && !isGeneratedImageAltText(safeAlt)
        ? `\n${safeAlt}\n`
        : '\n';
    })
    .trim();

  if (images.length === 0) {
    return normalized;
  }

  return [
    ...(text
      ? [
          {
            type: 'text',
            text,
          },
        ]
      : []),
    ...images,
  ];
};

const getStreamingDisplayContent = (rawContent) =>
  hasInlineDataImagePayload(rawContent)
    ? '正在接收图片数据，完成后会显示可放大和下载的图片预览...'
    : rawContent;

const buildImageTaskContentUrl = (taskId, index) =>
  `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}/image/${index}`;

const extractChatGPTWebConversationId = (data) => {
  const direct = data?.conversation_id || data?.conversationId;
  if (typeof direct === 'string' && direct.trim()) {
    return direct.trim();
  }
  const id = data?.id;
  const prefix = 'chatcmpl-chatgptimg-';
  if (typeof id === 'string' && id.startsWith(prefix)) {
    return id.slice(prefix.length).trim();
  }
  return '';
};

const imageTaskResultToMessageContent = (taskId, taskResult) => {
  const resultData = taskResult?.data ?? taskResult;
  const items = Array.isArray(resultData?.data) ? resultData.data : [];
  const content = [];
  const textParts = [];

  items.forEach((item, index) => {
    const hasImage = Boolean(item?.url || item?.b64_json);
    const url = hasImage ? buildImageTaskContentUrl(taskId, index) : '';
    if (url) {
      content.push({
        type: 'image_url',
        image_url: { url },
      });
    }
    if (item?.revised_prompt) {
      textParts.push(`Revised prompt ${index + 1}: ${item.revised_prompt}`);
    }
  });

  if (textParts.length > 0) {
    content.unshift({
      type: 'text',
      text: textParts.join('\n\n'),
    });
  }

  if (content.length > 0) {
    return content;
  }

  const markdown = imageResponseToMarkdown(resultData);
  return (
    markdown ||
    `图片生成完成，但响应中没有可显示的图片数据。\n\n\`\`\`json\n${JSON.stringify(resultData, null, 2)}\n\`\`\``
  );
};

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const isTerminalImageTaskStatus = (status) => {
  const normalized = String(status || '').toLowerCase();
  return ['succeeded', 'failed', 'success', 'failure', 'completed'].includes(
    normalized,
  );
};

const isSuccessfulImageTaskStatus = (status) => {
  const normalized = String(status || '').toLowerCase();
  return ['succeeded', 'success', 'completed'].includes(normalized);
};

const formatImageGenerationElapsed = (seconds) => {
  if (seconds < 60) {
    return `${seconds} 秒`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return remainingSeconds > 0
    ? `${minutes} 分 ${remainingSeconds} 秒`
    : `${minutes} 分钟`;
};

const getImageGenerationTaskState = (
  taskId,
  submitData,
  existingTask = {},
) => ({
  ...existingTask,
  id: taskId,
  submitData: existingTask?.submitData || submitData || null,
  startedAt: existingTask?.startedAt || Date.now(),
  updatedAt: Date.now(),
});

const getImageGenerationElapsedSeconds = (attempt, startedAt) => {
  const parsedStartedAt = Number(startedAt);
  if (parsedStartedAt > 0) {
    return Math.max(0, Math.floor((Date.now() - parsedStartedAt) / 1000));
  }
  return Math.max(0, attempt + 1) * 5;
};

const getImageGenerationWaitMessage = (
  taskId,
  taskData,
  attempt,
  startedAt,
) => {
  const safeAttempt = Math.max(0, attempt);
  const elapsedSeconds = getImageGenerationElapsedSeconds(attempt, startedAt);
  const elapsed = formatImageGenerationElapsed(elapsedSeconds);
  const rawProgress = String(taskData?.progress || '').trim();
  const shouldShowRawProgress = rawProgress && rawProgress !== '1%';
  const dots = '.'.repeat((safeAttempt % 3) + 1);

  let stage = '已提交任务，正在等待 ChatGPT 开始生图';
  if (elapsedSeconds >= 20) {
    stage = 'ChatGPT 正在生成图片，通常需要 1-3 分钟';
  }
  if (elapsedSeconds >= 90) {
    stage = '图片仍在生成中，上游本次响应偏慢';
  }
  if (elapsedSeconds >= 240) {
    stage = '继续等待上游返回结果，最长可等待约 20 分钟';
  }

  return [
    `图片生成中${dots}`,
    '',
    `**状态：** ${stage}`,
    `**已等待：** ${elapsed}`,
    shouldShowRawProgress ? `**上游进度：** ${rawProgress}` : '',
    '',
    `任务 ID：\`${taskId}\``,
    '结果完成后会自动显示，可点击图片放大或下载。',
  ]
    .filter(Boolean)
    .join('\n');
};

export const useApiRequest = (
  setMessage,
  setDebugData,
  setActiveDebugTab,
  sseSourceRef,
  saveMessages,
  onConversationId,
) => {
  const { t } = useTranslation();
  const imageSubmitKeysRef = useRef(new Set());
  const imageTaskPollsRef = useRef(new Set());

  // 处理消息自动关闭逻辑的公共函数
  const applyAutoCollapseLogic = useCallback(
    (message, isThinkingComplete = true) => {
      const shouldAutoCollapse =
        isThinkingComplete && !message.hasAutoCollapsed;
      return {
        isThinkingComplete,
        hasAutoCollapsed: shouldAutoCollapse || message.hasAutoCollapsed,
        isReasoningExpanded: shouldAutoCollapse
          ? false
          : message.isReasoningExpanded,
      };
    },
    [],
  );

  // 流式消息更新
  const streamMessageUpdate = useCallback(
    (textChunk, type) => {
      setMessage((prevMessage) => {
        const lastMessage = prevMessage[prevMessage.length - 1];
        if (!lastMessage) return prevMessage;
        if (lastMessage.role !== 'assistant') return prevMessage;
        if (lastMessage.status === MESSAGE_STATUS.ERROR) {
          return prevMessage;
        }

        if (
          lastMessage.status === MESSAGE_STATUS.LOADING ||
          lastMessage.status === MESSAGE_STATUS.INCOMPLETE
        ) {
          let newMessage = { ...lastMessage };

          if (type === 'reasoning') {
            newMessage = {
              ...newMessage,
              reasoningContent:
                (lastMessage.reasoningContent || '') + textChunk,
              status: MESSAGE_STATUS.INCOMPLETE,
              isThinkingComplete: false,
            };
          } else if (type === 'content') {
            const shouldCollapseReasoning =
              !lastMessage.content && lastMessage.reasoningContent;
            const previousContent =
              typeof lastMessage.rawContent === 'string'
                ? lastMessage.rawContent
                : lastMessage.content || '';
            const newContent = previousContent + textChunk;
            const displayContent = getStreamingDisplayContent(newContent);

            let shouldCollapseFromThinkTag = false;
            let thinkingCompleteFromTags = lastMessage.isThinkingComplete;

            if (
              lastMessage.isReasoningExpanded &&
              newContent.includes('</think>')
            ) {
              const thinkMatches = newContent.match(/<think>/g);
              const thinkCloseMatches = newContent.match(/<\/think>/g);
              if (
                thinkMatches &&
                thinkCloseMatches &&
                thinkCloseMatches.length >= thinkMatches.length
              ) {
                shouldCollapseFromThinkTag = true;
                thinkingCompleteFromTags = true; // think标签闭合也标记思考完成
              }
            }

            // 如果开始接收content内容，且之前有reasoning内容，或者think标签已闭合，则标记思考完成
            const isThinkingComplete =
              (lastMessage.reasoningContent &&
                !lastMessage.isThinkingComplete) ||
              thinkingCompleteFromTags;

            const autoCollapseState = applyAutoCollapseLogic(
              lastMessage,
              isThinkingComplete,
            );

            newMessage = {
              ...newMessage,
              content: displayContent,
              rawContent: displayContent === newContent ? undefined : newContent,
              status: MESSAGE_STATUS.INCOMPLETE,
              ...autoCollapseState,
            };
          }

          return [...prevMessage.slice(0, -1), newMessage];
        }

        return prevMessage;
      });
    },
    [setMessage, applyAutoCollapseLogic],
  );

  // 完成消息
  const completeMessage = useCallback(
    (status = MESSAGE_STATUS.COMPLETE) => {
      setMessage((prevMessage) => {
        const lastMessage = prevMessage[prevMessage.length - 1];
        if (
          lastMessage.status === MESSAGE_STATUS.COMPLETE ||
          lastMessage.status === MESSAGE_STATUS.ERROR
        ) {
          return prevMessage;
        }

        const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

        const updatedMessages = [
          ...prevMessage.slice(0, -1),
          {
            ...lastMessage,
            content:
              status === MESSAGE_STATUS.COMPLETE
                ? markdownImagesToMessageContent(
                    lastMessage.rawContent || lastMessage.content,
                  )
                : lastMessage.content,
            rawContent: undefined,
            status: status,
            ...autoCollapseState,
          },
        ];

        // 在消息完成时保存，传入更新后的消息列表
        if (
          status === MESSAGE_STATUS.COMPLETE ||
          status === MESSAGE_STATUS.ERROR
        ) {
          setTimeout(() => saveMessages(updatedMessages), 0);
        }

        return updatedMessages;
      });
    },
    [setMessage, applyAutoCollapseLogic, saveMessages],
  );

  const updatePendingImageGenerationMessage = useCallback(
    (taskId, submitData, attempt, options = {}) => {
      setMessage((prevMessage) => {
        const targetIndex = options.messageId
          ? prevMessage.findIndex((msg) => msg.id === options.messageId)
          : prevMessage.length - 1;

        if (targetIndex < 0) return prevMessage;

        const targetMessage = prevMessage[targetIndex];
        if (
          targetMessage?.status !== MESSAGE_STATUS.LOADING &&
          targetMessage?.status !== MESSAGE_STATUS.INCOMPLETE
        ) {
          return prevMessage;
        }

        const imageGenerationTask = getImageGenerationTaskState(
          taskId,
          submitData,
          {
            ...(targetMessage.imageGenerationTask || {}),
            startedAt:
              options.startedAt || targetMessage.imageGenerationTask?.startedAt,
          },
        );
        const updatedMessages = [...prevMessage];
        updatedMessages[targetIndex] = {
          ...targetMessage,
          content: getImageGenerationWaitMessage(
            taskId,
            submitData,
            attempt,
            imageGenerationTask.startedAt,
          ),
          status: MESSAGE_STATUS.LOADING,
          imageGenerationTask,
        };

        setTimeout(() => saveMessages(updatedMessages), 0);
        return updatedMessages;
      });
    },
    [setMessage, saveMessages],
  );

  const pollImageGenerationTask = useCallback(
    async (taskId, submitData, options = {}) => {
      const endpoint = `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}`;
      const maxAttempts = 240;

      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        await sleep(5000);

        const response = await fetch(endpoint, {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
            'New-Api-User': getUserIdFromLocalStorage(),
          },
        });

        if (!response.ok) {
          const errorBody = await response.text();
          throw new Error(
            `图片任务轮询失败: ${response.status}, body: ${errorBody}`,
          );
        }

        const taskData = await response.json();
        const conversationId = extractChatGPTWebConversationId(taskData?.data);
        if (conversationId && onConversationId) {
          onConversationId(conversationId, {
            submitData,
            taskData,
            requestContext: submitData?.requestContext || null,
            isImagePayload: true,
          });
        }
        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(
            {
              submit: submitData,
              poll: taskData,
            },
            null,
            2,
          ),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        const status = String(taskData?.status || '').toLowerCase();
        if (!isTerminalImageTaskStatus(status)) {
          updatePendingImageGenerationMessage(taskId, taskData, attempt, {
            ...options,
          });
          continue;
        }

        if (isSuccessfulImageTaskStatus(status)) {
          return taskData;
        }

        throw new Error(taskData?.fail_reason || '图片生成失败');
      }

      throw new Error('图片生成任务轮询超时');
    },
    [
      setActiveDebugTab,
      setDebugData,
      updatePendingImageGenerationMessage,
      onConversationId,
    ],
  );

  const completeImageGenerationMessage = useCallback(
    (content, options = {}) => {
      const processed =
        typeof content === 'string' ? processThinkTags(content, '') : null;

      setMessage((prevMessage) => {
        const targetIndex = options.messageId
          ? prevMessage.findIndex((msg) => msg.id === options.messageId)
          : prevMessage.length - 1;

        if (targetIndex < 0) return prevMessage;

        const targetMessage = prevMessage[targetIndex];
        if (
          targetMessage?.status !== MESSAGE_STATUS.LOADING &&
          targetMessage?.status !== MESSAGE_STATUS.INCOMPLETE
        ) {
          return prevMessage;
        }

        const autoCollapseState = applyAutoCollapseLogic(targetMessage, true);
        const updatedMessages = [...prevMessage];
        updatedMessages[targetIndex] = {
          ...targetMessage,
          content: processed ? processed.content : content,
          reasoningContent: processed?.reasoningContent || '',
          status: MESSAGE_STATUS.COMPLETE,
          imageGenerationTask: undefined,
          ...autoCollapseState,
        };

        setTimeout(() => saveMessages(updatedMessages), 0);
        return updatedMessages;
      });
    },
    [setMessage, applyAutoCollapseLogic, saveMessages],
  );

  const failImageGenerationMessage = useCallback(
    (error, options = {}) => {
      setMessage((prevMessage) => {
        const targetIndex = options.messageId
          ? prevMessage.findIndex((msg) => msg.id === options.messageId)
          : prevMessage.length - 1;

        if (targetIndex < 0) return prevMessage;

        const targetMessage = prevMessage[targetIndex];
        if (
          targetMessage?.status !== MESSAGE_STATUS.LOADING &&
          targetMessage?.status !== MESSAGE_STATUS.INCOMPLETE
        ) {
          return prevMessage;
        }

        const autoCollapseState = applyAutoCollapseLogic(targetMessage, true);
        const updatedMessages = [...prevMessage];
        updatedMessages[targetIndex] = {
          ...targetMessage,
          content: t('请求发生错误: ') + error.message,
          errorCode: error.errorCode || null,
          status: MESSAGE_STATUS.ERROR,
          imageGenerationTask: undefined,
          ...autoCollapseState,
        };

        setTimeout(() => saveMessages(updatedMessages), 0);
        return updatedMessages;
      });
    },
    [setMessage, applyAutoCollapseLogic, saveMessages, t],
  );

  const finishImageGenerationTask = useCallback(
    async ({ taskId, submitData, messageId, startedAt }) => {
      if (imageTaskPollsRef.current.has(taskId)) {
        return;
      }
      imageTaskPollsRef.current.add(taskId);
      try {
        const taskResult = await pollImageGenerationTask(taskId, submitData, {
          messageId,
          startedAt,
        });
        const content = imageTaskResultToMessageContent(taskId, taskResult);
        completeImageGenerationMessage(content, { messageId });
      } finally {
        imageTaskPollsRef.current.delete(taskId);
      }
    },
    [pollImageGenerationTask, completeImageGenerationMessage],
  );

  const resumeImageGenerationTask = useCallback(
    (task) => {
      const taskId = task?.taskId || task?.id;
      if (!taskId) return;

      updatePendingImageGenerationMessage(taskId, task.submitData || null, -1, {
        messageId: task.messageId,
        startedAt: task.startedAt,
      });

      finishImageGenerationTask({
        taskId,
        submitData: task.submitData || null,
        messageId: task.messageId,
        startedAt: task.startedAt,
      }).catch((error) => {
        console.error('Resume image generation task error:', error);
        const errorInfo = handleApiError(error);
        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(errorInfo, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);
        failImageGenerationMessage(error, { messageId: task.messageId });
      });
    },
    [
      updatePendingImageGenerationMessage,
      finishImageGenerationTask,
      failImageGenerationMessage,
      setDebugData,
      setActiveDebugTab,
    ],
  );

  // 非流式请求
  const handleNonStreamRequest = useCallback(
    async (payload) => {
      const isImagePayload = isImageGenerationPayload(payload);
      const imageSubmitKey = isImagePayload ? JSON.stringify(payload) : '';
      if (imageSubmitKey && imageSubmitKeysRef.current.has(imageSubmitKey)) {
        console.warn('Duplicate image generation submit blocked');
        return;
      }
      if (imageSubmitKey) {
        imageSubmitKeysRef.current.add(imageSubmitKey);
      }

      setDebugData((prev) => ({
        ...prev,
        request: payload,
        timestamp: new Date().toISOString(),
        response: null,
        sseMessages: null, // 非流式请求清除 SSE 消息
        isStreaming: false,
      }));
      setActiveDebugTab(DEBUG_TABS.REQUEST);

      try {
        const endpoint = getEndpointForPayload(payload);
        const response = await fetch(endpoint, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'New-Api-User': getUserIdFromLocalStorage(),
          },
          body: JSON.stringify(payload),
        });

        if (!response.ok) {
          const errorBody = await response.text();
          let parsedError = null;
          try {
            const errorJson = JSON.parse(errorBody);
            parsedError = errorJson?.error || null;
          } catch (_) {
            // noop
          }
          const errorInfo = handleApiError(
            new Error(
              `HTTP error! status: ${response.status}, body: ${errorBody}`,
            ),
            response,
          );
          setDebugData((prev) => ({
            ...prev,
            response: JSON.stringify(errorInfo, null, 2),
          }));
          setActiveDebugTab(DEBUG_TABS.RESPONSE);

          const err = new Error(
            parsedError?.message ||
              `HTTP error! status: ${response.status}, body: ${errorBody}`,
          );
          err.errorCode = parsedError?.code || null;
          err.errorType = parsedError?.type || null;
          throw err;
        }

        const submitData = await response.json();
        const conversationId = extractChatGPTWebConversationId(submitData);
        if (conversationId && onConversationId) {
          onConversationId(conversationId, {
            submitData,
            isImagePayload,
            requestContext: {
              model: payload.model,
              group: payload.group,
              kind: isImagePayload ? 'image' : 'chat',
            },
          });
        }

        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(submitData, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        const imageTaskId =
          submitData?.task_id || submitData?.taskId || submitData?.id;

        if (isImagePayload && imageTaskId) {
          const imageTaskSubmitData = {
            ...submitData,
            requestContext: {
              model: payload.model,
              group: payload.group,
              kind: 'image',
            },
          };
          updatePendingImageGenerationMessage(
            imageTaskId,
            imageTaskSubmitData,
            -1,
          );
          await finishImageGenerationTask({
            taskId: imageTaskId,
            submitData: imageTaskSubmitData,
            requestContext: imageTaskSubmitData.requestContext,
          });
          return;
        }

        if (submitData.choices?.[0] || Array.isArray(submitData.data)) {
          const choice = submitData.choices?.[0];
          let content = choice
            ? choice.message?.content || ''
            : imageResponseToMarkdown(submitData);
          let reasoningContent = choice
            ? choice.message?.reasoning_content ||
              choice.message?.reasoning ||
              ''
            : '';

          const processed = processThinkTags(content, reasoningContent);
          const finalContent = markdownImagesToMessageContent(
            processed.content,
          );

          setMessage((prevMessage) => {
            const newMessages = [...prevMessage];
            const lastMessage = newMessages[newMessages.length - 1];
            if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
              const autoCollapseState = applyAutoCollapseLogic(
                lastMessage,
                true,
              );

              newMessages[newMessages.length - 1] = {
                ...lastMessage,
                content: finalContent,
                reasoningContent: processed.reasoningContent,
                status: MESSAGE_STATUS.COMPLETE,
                rawContent: undefined,
                ...autoCollapseState,
              };
              setTimeout(() => saveMessages(newMessages), 0);
            }
            return newMessages;
          });
        }
      } catch (error) {
        console.error('Non-stream request error:', error);

        const errorInfo = handleApiError(error);
        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(errorInfo, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        setMessage((prevMessage) => {
          const newMessages = [...prevMessage];
          const lastMessage = newMessages[newMessages.length - 1];
          if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
            const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

            newMessages[newMessages.length - 1] = {
              ...lastMessage,
              content: t('请求发生错误: ') + error.message,
              errorCode: error.errorCode || null,
              status: MESSAGE_STATUS.ERROR,
              imageGenerationTask: undefined,
              ...autoCollapseState,
            };
            setTimeout(() => saveMessages(newMessages), 0);
          }
          return newMessages;
        });
      } finally {
        if (imageSubmitKey) {
          imageSubmitKeysRef.current.delete(imageSubmitKey);
        }
      }
    },
    [
      setDebugData,
      setActiveDebugTab,
      setMessage,
      t,
      applyAutoCollapseLogic,
      updatePendingImageGenerationMessage,
      finishImageGenerationTask,
      saveMessages,
      onConversationId,
    ],
  );

  // SSE请求
  const handleSSE = useCallback(
    (payload) => {
      const requestPayload = payload;
      setDebugData((prev) => ({
        ...prev,
        request: payload,
        timestamp: new Date().toISOString(),
        response: null,
        sseMessages: [], // 新增：存储 SSE 消息数组
        isStreaming: true, // 新增：标记流式状态
      }));
      setActiveDebugTab(DEBUG_TABS.REQUEST);

      const source = new SSE(getEndpointForPayload(payload), {
        headers: {
          'Content-Type': 'application/json',
          'New-Api-User': getUserIdFromLocalStorage(),
        },
        method: 'POST',
        payload: JSON.stringify(payload),
      });

      sseSourceRef.current = source;

      let responseData = '';
      let hasReceivedFirstResponse = false;
      let isStreamComplete = false; // 添加标志位跟踪流是否正常完成

      source.addEventListener('message', (e) => {
        if (e.data === '[DONE]') {
          isStreamComplete = true; // 标记流正常完成
          source.close();
          sseSourceRef.current = null;
          setDebugData((prev) => ({
            ...prev,
            response: responseData,
            sseMessages: [...(prev.sseMessages || []), '[DONE]'], // 添加 DONE 标记
            isStreaming: false,
          }));
          completeMessage();
          return;
        }

        try {
          const responsePayload = JSON.parse(e.data);
          const conversationId =
            extractChatGPTWebConversationId(responsePayload);
          if (conversationId && onConversationId) {
            onConversationId(conversationId, {
              requestContext: {
                model: requestPayload.model,
                group: requestPayload.group,
                kind: isImageGenerationPayload(requestPayload)
                  ? 'image'
                  : 'chat',
              },
              isImagePayload:
                isImageGenerationPayload(requestPayload),
            });
          }
          responseData += e.data + '\n';

          if (!hasReceivedFirstResponse) {
            setActiveDebugTab(DEBUG_TABS.RESPONSE);
            hasReceivedFirstResponse = true;
          }

          // 新增：将 SSE 消息添加到数组
          setDebugData((prev) => ({
            ...prev,
            sseMessages: [...(prev.sseMessages || []), e.data],
          }));

          const delta = responsePayload.choices?.[0]?.delta;
          if (delta) {
            if (delta.reasoning_content) {
              streamMessageUpdate(delta.reasoning_content, 'reasoning');
            }
            if (delta.reasoning) {
              streamMessageUpdate(delta.reasoning, 'reasoning');
            }
            if (delta.content) {
              streamMessageUpdate(delta.content, 'content');
            }
          }
        } catch (error) {
          console.error('Failed to parse SSE message:', error);
          const errorInfo = `解析错误: ${error.message}`;

          setDebugData((prev) => ({
            ...prev,
            response: responseData + `\n\nError: ${errorInfo}`,
            sseMessages: [...(prev.sseMessages || []), e.data], // 即使解析失败也保存原始数据
            isStreaming: false,
          }));
          setActiveDebugTab(DEBUG_TABS.RESPONSE);

          streamMessageUpdate(t('解析响应数据时发生错误'), 'content');
          completeMessage(MESSAGE_STATUS.ERROR);
        }
      });

      source.addEventListener('error', (e) => {
        // 只有在流没有正常完成且连接状态异常时才处理错误
        if (!isStreamComplete && source.readyState !== 2) {
          console.error('SSE Error:', e);
          let errorMessage = e.data || t('请求发生错误');
          let errorCode = null;

          if (e.data) {
            try {
              const errorJson = JSON.parse(e.data);
              if (errorJson?.error) {
                errorMessage = errorJson.error.message || errorMessage;
                errorCode = errorJson.error.code || null;
              }
            } catch (_) {
              // not JSON, use raw data as error message
            }
          }

          const errorInfo = handleApiError(new Error(errorMessage));
          errorInfo.readyState = source.readyState;

          setDebugData((prev) => ({
            ...prev,
            response:
              responseData +
              '\n\nSSE Error:\n' +
              JSON.stringify(errorInfo, null, 2),
          }));
          setActiveDebugTab(DEBUG_TABS.RESPONSE);

          setMessage((prevMessage) => {
            const newMessages = [...prevMessage];
            const lastMessage = newMessages[newMessages.length - 1];
            if (
              lastMessage &&
              lastMessage.status !== MESSAGE_STATUS.COMPLETE &&
              lastMessage.status !== MESSAGE_STATUS.ERROR
            ) {
              newMessages[newMessages.length - 1] = {
                ...lastMessage,
                content: (lastMessage.content || '') + errorMessage,
                errorCode: errorCode,
                status: MESSAGE_STATUS.ERROR,
              };
            }
            return newMessages;
          });
          sseSourceRef.current = null;
          source.close();
        }
      });

      source.addEventListener('readystatechange', (e) => {
        // 检查 HTTP 状态错误，但避免与正常关闭重复处理
        if (
          e.readyState >= 2 &&
          source.status !== undefined &&
          source.status !== 200 &&
          !isStreamComplete
        ) {
          const errorInfo = handleApiError(new Error('HTTP状态错误'));
          errorInfo.status = source.status;
          errorInfo.readyState = source.readyState;

          setDebugData((prev) => ({
            ...prev,
            response:
              responseData +
              '\n\nHTTP Error:\n' +
              JSON.stringify(errorInfo, null, 2),
          }));
          setActiveDebugTab(DEBUG_TABS.RESPONSE);

          source.close();
          streamMessageUpdate(t('连接已断开'), 'content');
          completeMessage(MESSAGE_STATUS.ERROR);
        }
      });

      try {
        source.stream();
      } catch (error) {
        console.error('Failed to start SSE stream:', error);
        const errorInfo = handleApiError(error);

        setDebugData((prev) => ({
          ...prev,
          response: 'Stream启动失败:\n' + JSON.stringify(errorInfo, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        streamMessageUpdate(t('建立连接时发生错误'), 'content');
        completeMessage(MESSAGE_STATUS.ERROR);
      }
    },
    [
      setDebugData,
      setActiveDebugTab,
      setMessage,
      streamMessageUpdate,
      completeMessage,
      t,
      applyAutoCollapseLogic,
      onConversationId,
    ],
  );

  // 停止生成
  const onStopGenerator = useCallback(() => {
    // 如果仍有活动的 SSE 连接，首先关闭
    if (sseSourceRef.current) {
      sseSourceRef.current.close();
      sseSourceRef.current = null;
    }

    // 无论是否存在 SSE 连接，都尝试处理最后一条正在生成的消息
    setMessage((prevMessage) => {
      if (prevMessage.length === 0) return prevMessage;
      const lastMessage = prevMessage[prevMessage.length - 1];

      if (
        lastMessage.status === MESSAGE_STATUS.LOADING ||
        lastMessage.status === MESSAGE_STATUS.INCOMPLETE
      ) {
        const processed = processIncompleteThinkTags(
          lastMessage.content || '',
          lastMessage.reasoningContent || '',
        );

        const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

        const updatedMessages = [
          ...prevMessage.slice(0, -1),
          {
            ...lastMessage,
            status: MESSAGE_STATUS.COMPLETE,
            reasoningContent: processed.reasoningContent || null,
            content: processed.content,
            imageGenerationTask: undefined,
            ...autoCollapseState,
          },
        ];

        // 停止生成时也保存，传入更新后的消息列表
        setTimeout(() => saveMessages(updatedMessages), 0);

        return updatedMessages;
      }
      return prevMessage;
    });
  }, [setMessage, applyAutoCollapseLogic, saveMessages]);

  // 发送请求
  const sendRequest = useCallback(
    (payload, isStream) => {
      const normalizedPayload = normalizePlaygroundPayloadForTransport(payload);

      if (isImageGenerationPayload(normalizedPayload) || !isStream) {
        handleNonStreamRequest(normalizedPayload);
      } else {
        handleSSE(normalizedPayload);
      }
    },
    [handleSSE, handleNonStreamRequest],
  );

  return {
    sendRequest,
    onStopGenerator,
    streamMessageUpdate,
    completeMessage,
    resumeImageGenerationTask,
  };
};
