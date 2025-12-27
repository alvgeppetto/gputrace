import { TraceItem, NodeType } from './types';
import { MarkerType } from 'reactflow';

export const MOCK_TRACE_DATA: TraceItem[] = [
  {
    id: 'root-summary',
    label: 'mlx-lm-generate_tokens_8_to_9',
    type: NodeType.ROOT,
    stats: { duration: '1.86 ms', memory: '942.97 MiB', threads: '1' },
    children: [
      {
        id: 'group-squeeze',
        label: 'Squeeze',
        type: NodeType.ROOT, // Using ROOT/Group type for container
        stats: { duration: '0.23%', memory: '-', threads: '-' },
        children: [
          {
            id: 'enc-squeeze',
            label: 'Compute Encoder 0 0xa08c485a0',
            type: NodeType.ENCODER,
            description: 'Compute Encoder 0',
            stats: { duration: '0.23%', memory: '-', threads: '65536' },
            properties: {
              'Encoder Address': '0xa08c485a0',
              'Label': 'Squeeze',
              'Device': 'Apple M3 Max',
            },
            children: [
              {
                id: 'disp-60',
                label: '60 [dispatchThreads:(65536, 1, 1)]',
                type: NodeType.DISPATCH,
                stats: { duration: '0.42%', memory: '', threads: '(65536, 1, 1)' },
                properties: {
                  'Function': 'fill_1',
                  'Grid Size': '(65536, 1, 1)',
                  'Threadgroup Size': '(1024, 1, 1)',
                  'Pipeline State': '0xa09d38900',
                },
                inputs: [
                  { id: 'buf-1', label: 'Buffer 0xa09de1f80', type: 'buffer', size: '2.00 KiB', access: 'Read', status: 'Tracked' }
                ],
                outputs: [
                  { id: 'buf-2', label: 'Buffer 0xa09de2bc0', type: 'buffer', size: '2.00 KiB', access: 'Write', status: 'Shared' }
                ]
              },
            ]
          }
        ]
      },
      {
        id: 'enc-quantized',
        label: 'QuantizedMatmul',
        type: NodeType.ENCODER,
        description: 'Compute Encoder 0 0xa08c49040',
        stats: { duration: '6.53%', memory: '-', threads: '65536' },
        properties: {
          'Encoder Address': '0xa08c49040',
          'Label': 'QuantizedMatmul',
        },
        children: [
          {
            id: 'disp-645',
            label: '645 [dispatchThreads:(65536, 1, 1)]',
            type: NodeType.DISPATCH,
            stats: { duration: '0.28%', memory: '', threads: '(65536, 1, 1)' },
            properties: {
              'Function': 'gemm_quantized',
              'Grid Size': '(65536, 1, 1)',
              'Threadgroup Size': '(32, 32, 1)',
            },
            inputs: [
              { id: 'buf-2', label: 'Buffer 0xa09de2bc0', type: 'buffer', size: '2.00 KiB', access: 'Read', status: 'Shared' }
            ],
            outputs: [
              { id: 'buf-3', label: 'Buffer 0xa09dea140', type: 'buffer', size: '512.00 KiB', access: 'Write', status: 'Shared' }
            ]
          },
          {
            id: 'barrier-1',
            label: 'Barrier',
            type: NodeType.BARRIER,
            stats: { duration: '0.01%', memory: '-', threads: '-' }
          }
        ]
      },
      {
        id: 'enc-reshape',
        label: 'Reshape',
        type: NodeType.ENCODER,
        description: 'Compute Encoder 0 0xa08c49180',
        stats: { duration: '5.68%', memory: '-', threads: '65536' },
        children: [
          {
            id: 'disp-1239',
            label: '1239 [dispatchThreads]',
            type: NodeType.DISPATCH,
            properties: {
              'Function': 'reshape_tensor',
              'Grid Size': '(65536, 1, 1)',
            }
          }
        ]
      },
      {
        id: 'enc-rmsnorm',
        label: 'RMSNorm',
        type: NodeType.ENCODER,
        description: 'Compute Encoder 0 0xa08c49360',
        stats: { duration: '5.41%', memory: '-', threads: '65536' },
        children: []
      }
    ]
  }
];

// Initial nodes for React Flow
export const INITIAL_NODES = [
  // Group 1: Squeeze
  {
    id: 'enc-squeeze',
    type: 'group',
    data: { label: 'Compute Encoder 0 0xa08c485a0' },
    position: { x: 250, y: 0 },
    style: { width: 300, height: 200, backgroundColor: 'rgba(30, 30, 30, 0.4)', borderColor: '#ef4444', borderWidth: 1, borderRadius: 8 },
  },
  {
    id: 'disp-60',
    type: 'dispatch',
    data: {
      label: 'Dispatch 60',
      function: 'fill_1',
      duration: '0.42%',
      threads: '65536',
      grid: '(64k, 1, 1)'
    },
    position: { x: 50, y: 60 },
    parentNode: 'enc-squeeze',
    extent: 'parent',
  },

  // Group 2: QuantizedMatmul
  {
    id: 'enc-quantized',
    type: 'group',
    data: { label: 'QuantizedMatmul' },
    position: { x: 250, y: 300 },
    style: { width: 300, height: 400, backgroundColor: 'rgba(30, 30, 30, 0.4)', borderColor: '#ef4444', borderWidth: 1, borderRadius: 8 },
  },
  {
    id: 'disp-645',
    type: 'dispatch',
    data: {
      label: 'Dispatch 645',
      function: 'gemm_quantized',
      duration: '0.28%',
      threads: '65536',
      grid: '(64k, 1, 1)'
    },
    position: { x: 50, y: 60 },
    parentNode: 'enc-quantized',
    extent: 'parent',
  },
  {
    id: 'barrier-1',
    type: 'barrier',
    data: { label: 'Barrier', type: 'Memory' },
    position: { x: 75, y: 180 },
    parentNode: 'enc-quantized',
    extent: 'parent',
  },
  {
    id: 'disp-657',
    type: 'dispatch',
    data: {
      label: 'Dispatch 657',
      function: 'gemm_finish',
      duration: '<0.01%',
      threads: '1024',
      grid: '(1024, 1, 1)'
    },
    position: { x: 50, y: 280 },
    parentNode: 'enc-quantized',
    extent: 'parent',
  },

  // Orphan buffer nodes for visualization
  // Changed ID from buf-vis-1 to buf-1 to match MOCK_TRACE_DATA
  {
    id: 'buf-1',
    type: 'buffer',
    data: { label: 'Buffer 0xa0...1f80', size: '2KB', type: 'Shared' },
    position: { x: 600, y: 100 }
  }
];

export const INITIAL_EDGES = [
  { id: 'e1-2', source: 'disp-60', target: 'disp-645', animated: true, style: { stroke: '#3b82f6', strokeWidth: 2 }, markerEnd: { type: MarkerType.ArrowClosed, color: '#3b82f6' } },
  { id: 'e2-3', source: 'disp-645', target: 'barrier-1', style: { stroke: '#52525b' } },
  { id: 'e3-4', source: 'barrier-1', target: 'disp-657', style: { stroke: '#52525b' } },

  // Cross group buffer dependency
  // Changed source from buf-vis-1 to buf-1
  { id: 'e-buf-1', source: 'buf-1', target: 'disp-60', animated: false, style: { stroke: '#94a3b8', strokeDasharray: '5,5' } }
];

// Mock data for Timeline
export const TIMELINE_TRACKS = [
  { id: 't1', label: 'Vertex', type: 'encoder' },
  { id: 't2', label: 'Fragment', type: 'encoder' },
  { id: 't3', label: 'Compute', type: 'encoder' },
  { id: 't4', label: 'Compute Shaders', type: 'compute' },
];

export const TIMELINE_EVENTS = [
  { id: 'ev1', trackId: 't3', label: 'Compute Encoder 0', start: 0.1, duration: 1.2, color: 'bg-yellow-600/50' },
  { id: 'ev2', trackId: 't4', label: 'affine_qmv_fast_bf16', start: 0.15, duration: 0.3, color: 'bg-yellow-500' },
  { id: 'ev3', trackId: 't4', label: 'rms_norm_bf16', start: 0.5, duration: 0.2, color: 'bg-yellow-500' },
  { id: 'ev4', trackId: 't4', label: 'vn_copy_bf16', start: 0.75, duration: 0.1, color: 'bg-yellow-500' },
  { id: 'ev5', trackId: 't4', label: 'rope_single_bf16', start: 0.9, duration: 0.2, color: 'bg-yellow-500' },
  { id: 'ev6', trackId: 't4', label: 'gg2_copy_bf16', start: 1.15, duration: 0.1, color: 'bg-yellow-500' },
];

// Mock data for Cost Graph
export const COST_DATA = [
  { name: 'affine_qmv_fast_bfloat16_t_gs_64', cost: 14.08, color: '#3b82f6' },
  { name: 'vn_LogFloat32float', cost: 0.44, color: '#f59e0b' },
  { name: 'vn_copybfloat16', cost: 0.41, color: '#10b981' },
  { name: 'vn_copybfloat16_2', cost: 0.34, color: '#3b82f6' },
  { name: 'rmsbfloat16', cost: 0.31, color: '#ef4444' },
  { name: 'sdpa_vector_bfloat16', cost: 0.29, color: '#8b5cf6' },
  { name: 'svn_Multiplyfloat', cost: 0.26, color: '#ec4899' },
  { name: 'rbitsc', cost: 0.25, color: '#14b8a6' },
  { name: 'vn_copyuint32float', cost: 0.23, color: '#f97316' },
  { name: 'vsn_Dividefloat32', cost: 0.22, color: '#6366f1' },
  { name: 'vsn_Addfloat32', cost: 0.20, color: '#84cc16' },
];

export const MOCK_SOURCE_CODE = `// Copyright © 2024 Apple Inc.

template <typename T, typename U, int N = WorkPerThread<U>::n>
[[kernel]] void copy_s(
    device const T* src [[buffer(0)]],
    device U* dst [[buffer(1)]],
    constant uint& size,
    uint index [[thread_position_in_grid]]) {
  index *= N;
  if (N > 1 && index + N > size) {
    for (int i = 0; i < N; ++i) {
      if (index + i < size) {
         dst[index + i] = static_cast<U>(src[index + i]);
      }
    }
  } else {
    for (int i = 0; i < N; ++i) {
      dst[index + i] = static_cast<U>(src[index + i]);
    }
  }
}

template <typename T, typename U, int N = WorkPerThread<U>::n>
[[kernel]] void affine_qmv_fast(
    device const T* src [[buffer(0)]],
    // ... specialized logic for quantized matrix vector multiplication
    device U* dst [[buffer(1)]]) {

    // Simulating heavy compute load
    // ...
}`;
