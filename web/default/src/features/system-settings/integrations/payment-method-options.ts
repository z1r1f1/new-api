/*
Copyright (C) 2023-2026 QuantumNous

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
import { PAYMENT_ICON_COLORS, PAYMENT_TYPES } from '../../wallet/constants'

export type PaymentMethodTypeOption = {
  value: string
  label: string
}

export type PaymentMethodTemplate = {
  name: string
  template: {
    color: string
    min_topup?: string
    name: string
    type: string
  }
}

export const PAYMENT_METHOD_TYPE_OPTIONS: PaymentMethodTypeOption[] = [
  { value: PAYMENT_TYPES.ALIPAY, label: 'Alipay' },
  { value: PAYMENT_TYPES.WECHAT, label: 'WeChat Pay' },
  { value: PAYMENT_TYPES.STRIPE, label: 'Stripe' },
  { value: PAYMENT_TYPES.LINUXDO_CREDIT, label: 'Linux DO Credit' },
]

export const PAYMENT_METHOD_TEMPLATES: PaymentMethodTemplate[] = [
  {
    name: 'Alipay',
    template: {
      color: 'rgba(var(--semi-blue-5), 1)',
      name: '支付宝',
      type: PAYMENT_TYPES.ALIPAY,
    },
  },
  {
    name: 'WeChat Pay',
    template: {
      color: 'rgba(var(--semi-green-5), 1)',
      name: '微信',
      type: PAYMENT_TYPES.WECHAT,
    },
  },
  {
    name: 'Stripe',
    template: {
      color: 'rgba(var(--semi-green-5), 1)',
      name: 'Stripe',
      type: PAYMENT_TYPES.STRIPE,
    },
  },
  {
    name: 'Linux DO Credit',
    template: {
      color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.LINUXDO_CREDIT],
      min_topup: '1',
      name: 'Linux DO Credit',
      type: PAYMENT_TYPES.LINUXDO_CREDIT,
    },
  },
  {
    name: 'Custom',
    template: {
      color: 'black',
      min_topup: '50',
      name: '自定义1',
      type: 'custom1',
    },
  },
]
