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

import React, { useCallback } from 'react';
import { Modal, ImagePreview, Progress, Spin, Typography } from '@douyinfe/semi-ui';

const getImageDownloadName = (src = '') => {
  const cleanSrc = String(src).split('?')[0];
  const tail = cleanSrc.slice(cleanSrc.lastIndexOf('/') + 1);
  if (/\.(png|jpe?g|webp|gif)$/i.test(tail)) {
    return tail;
  }
  return `${tail || 'drawing-image'}.png`;
};

const withDownloadParam = (src = '') => {
  if (!src || src.startsWith('data:')) {
    return src;
  }
  const separator = src.includes('?') ? '&' : '?';
  return `${src}${separator}download=1`;
};

const ContentModal = ({
  isModalOpen,
  setIsModalOpen,
  modalContent,
  isModalOpenurl,
  setIsModalOpenurl,
  modalImageUrl,
  modalPreviewSrc,
  isModalImageLoading,
  modalImageLoadProgress,
}) => {
  const handleDownloadError = useCallback((src) => {
    const source = modalImageUrl || src;
    if (!source) return;
    const link = document.createElement('a');
    link.href = withDownloadParam(source);
    link.download = getImageDownloadName(source);
    document.body.appendChild(link);
    link.click();
    link.remove();
  }, [modalImageUrl]);

  return (
    <>
      {/* Text Content Modal */}
      <Modal
        visible={isModalOpen}
        onOk={() => setIsModalOpen(false)}
        onCancel={() => setIsModalOpen(false)}
        closable={null}
        bodyStyle={{ height: '400px', overflow: 'auto' }}
        width={800}
      >
        <p style={{ whiteSpace: 'pre-line' }}>{modalContent}</p>
      </Modal>

      {/* Image Preview Modal */}
      <ImagePreview
        src={modalPreviewSrc}
        visible={isModalOpenurl && !isModalImageLoading && !!modalPreviewSrc}
        setDownloadName={getImageDownloadName}
        onDownloadError={handleDownloadError}
        onVisibleChange={(visible) => setIsModalOpenurl(visible)}
      />

      <Modal
        visible={isModalOpenurl && isModalImageLoading}
        onCancel={() => setIsModalOpenurl(false)}
        footer={null}
        closable
        maskClosable
        width={560}
        centered
      >
        <div
          style={{
            minHeight: '240px',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '16px',
            padding: '24px 16px',
            textAlign: 'center',
          }}
        >
          <Spin size='large' />
          <Typography.Text type='tertiary'>
            正在加载图片，请稍候…
          </Typography.Text>
          <div style={{ width: '100%', maxWidth: '320px' }}>
            <Progress
              percent={Math.max(0, Math.min(100, modalImageLoadProgress || 0))}
              showInfo
              stroke='var(--semi-color-primary)'
              aria-label='image loading progress'
            />
          </div>
        </div>
      </Modal>
    </>
  );
};

export default ContentModal;
