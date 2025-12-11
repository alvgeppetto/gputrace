import { Node, Edge } from 'reactflow';

export enum NodeType {
  ENCODER = 'encoder',
  DISPATCH = 'dispatch',
  BARRIER = 'barrier',
  BUFFER = 'buffer',
  ROOT = 'root'
}

export interface TraceStats {
  duration: string;
  memory: string;
  threads: string;
}

export interface TraceItem {
  id: string;
  label: string;
  type: NodeType;
  description?: string;
  stats?: TraceStats;
  children?: TraceItem[];
  properties?: Record<string, any>;
  inputs?: ResourceRef[];
  outputs?: ResourceRef[];
}

export interface ResourceRef {
  id: string;
  label: string;
  type: 'buffer' | 'texture';
  size: string;
  access: 'Read' | 'Write' | 'Read/Write';
  status: 'Tracked' | 'Untracked' | 'Shared';
}

export type TraceNode = Node<TraceItem>;
export type TraceEdge = Edge;

export interface SelectionContext {
  selectedId: string | null;
  selectItem: (id: string | null) => void;
  traceData: TraceItem[];
}
