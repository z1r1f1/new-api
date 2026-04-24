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

import { useCallback } from 'react';
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

const buildImageTaskContentUrl = (taskId, index) =>
  `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}/image/${index}`;

const imageTaskResultToMessageContent = (taskId, taskResult) => {
  const resultData = taskResult?.data ?? taskResult;
  const items = Array.isArray(resultData?.data) ? resultData.data : [];
  const content = [];
  const textParts = [];

  items.forEach((item, index) => {
    const url =
      item?.url ||
      (item?.b64_json ? buildImageTaskContentUrl(taskId, index) : '');
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

const getImageGenerationWaitMessage = (taskId, taskData, attempt) => {
  const safeAttempt = Math.max(0, attempt);
  const elapsedSeconds = Math.max(0, attempt + 1) * 5;
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
) => {
  const { t } = useTranslation();

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
            const newContent = (lastMessage.content || '') + textChunk;

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
              content: newContent,
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

  const pollImageGenerationTask = useCallback(
    async (taskId, submitData) => {
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
          setMessage((prevMessage) => {
            const newMessages = [...prevMessage];
            const lastMessage = newMessages[newMessages.length - 1];
            if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
              newMessages[newMessages.length - 1] = {
                ...lastMessage,
                content: getImageGenerationWaitMessage(
                  taskId,
                  taskData,
                  attempt,
                ),
                status: MESSAGE_STATUS.LOADING,
              };
            }
            return newMessages;
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
    [setActiveDebugTab, setDebugData, setMessage],
  );

  // 非流式请求
  const handleNonStreamRequest = useCallback(
    async (payload) => {
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

        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(submitData, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        const imageTaskId =
          submitData?.task_id || submitData?.taskId || submitData?.id;

        if (isImageGenerationPayload(payload) && imageTaskId) {
          setMessage((prevMessage) => {
            const newMessages = [...prevMessage];
            const lastMessage = newMessages[newMessages.length - 1];
            if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
              newMessages[newMessages.length - 1] = {
                ...lastMessage,
                content: getImageGenerationWaitMessage(
                  imageTaskId,
                  submitData,
                  -1,
                ),
                status: MESSAGE_STATUS.LOADING,
              };
            }
            return newMessages;
          });

          const taskResult = await pollImageGenerationTask(
            imageTaskId,
            submitData,
          );
          const content = imageTaskResultToMessageContent(
            imageTaskId,
            taskResult,
          );
          const processed =
            typeof content === 'string' ? processThinkTags(content, '') : null;

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
                content: processed ? processed.content : content,
                reasoningContent: processed?.reasoningContent || '',
                status: MESSAGE_STATUS.COMPLETE,
                ...autoCollapseState,
              };
              setTimeout(() => saveMessages(newMessages), 0);
            }
            return newMessages;
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
                content: processed.content,
                reasoningContent: processed.reasoningContent,
                status: MESSAGE_STATUS.COMPLETE,
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
              ...autoCollapseState,
            };
            setTimeout(() => saveMessages(newMessages), 0);
          }
          return newMessages;
        });
      }
    },
    [
      setDebugData,
      setActiveDebugTab,
      setMessage,
      t,
      applyAutoCollapseLogic,
      pollImageGenerationTask,
      saveMessages,
    ],
  );

  // SSE请求
  const handleSSE = useCallback(
    (payload) => {
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
          const payload = JSON.parse(e.data);
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

          const delta = payload.choices?.[0]?.delta;
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
      if (isImageGenerationPayload(payload) || !isStream) {
        handleNonStreamRequest(payload);
      } else {
        handleSSE(payload);
      }
    },
    [handleSSE, handleNonStreamRequest],
  );

  return {
    sendRequest,
    onStopGenerator,
    streamMessageUpdate,
    completeMessage,
  };
};
