import React, { useState, useMemo } from 'react';
import { ChevronRight, ChevronDown, Layers, Calculator, Play, Ban, FileText, Search, Filter, Sidebar as SidebarIcon } from 'lucide-react';
import { TraceItem, NodeType } from '../types';

interface SidebarProps {
  onSelect: (item: TraceItem) => void;
  selectedId: string | null;
  hoveredId?: string | null;
  onHover?: (id: string | null) => void;
}

// TraceNodeItem Component
const TraceNodeItem: React.FC<{
  item: TraceItem;
  depth: number;
  onSelect: (i: TraceItem) => void;
  selectedId: string | null;
  searchTerm: string;
  alwaysExpand?: boolean;
  hoveredId?: string | null;
  onHover?: (id: string | null) => void;
}> = ({ item, depth, onSelect, selectedId, searchTerm, alwaysExpand, hoveredId, onHover }) => {
  const [expanded, setExpanded] = useState(depth < 1);
  const isSelected = selectedId === item.id;
  const isHovered = hoveredId === item.id;

  // Force expansion if search term matches children (passed down via alwaysExpand)
  // or if explicitly expanded by user
  const isExpanded = alwaysExpand || expanded;

  const getIcon = (type: NodeType) => {
    switch (type) {
      case NodeType.ROOT: return <FileText size={14} className="text-gray-400" />;
      case NodeType.ENCODER: return <Calculator size={14} className="text-orange-500" />;
      case NodeType.DISPATCH: return <Play size={14} className="text-blue-500" />;
      case NodeType.BARRIER: return <Ban size={14} className="text-red-400" />;
      default: return <Layers size={14} className="text-gray-400" />;
    }
  };

  // Highlight search term in label
  const renderLabel = () => {
    if (!searchTerm) return item.label;

    const index = item.label.toLowerCase().indexOf(searchTerm.toLowerCase());
    if (index === -1) return item.label;

    const before = item.label.substring(0, index);
    const match = item.label.substring(index, index + searchTerm.length);
    const after = item.label.substring(index + searchTerm.length);

    return (
      <>
        {before}
        <span className="bg-blue-900/60 text-blue-200 rounded px-0.5">{match}</span>
        {after}
      </>
    )
  };

  return (
    <div>
      <div
        className={`flex items-center py-1 px-2 cursor-pointer text-xs select-none ${isSelected ? 'bg-blue-900/40 border-l-2 border-blue-500' :
          isHovered ? 'bg-[#2a2d3e] border-l-2 border-blue-500/50' :
            'hover:bg-gray-800 border-l-2 border-transparent'
          }`}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
        onClick={() => {
          onSelect(item);
          // Auto-toggle expand if it has children
          if (item.children && item.children.length > 0 && !searchTerm) {
            setExpanded(!expanded);
          }
        }}
        onMouseEnter={() => onHover && onHover(item.id)}
        onMouseLeave={() => onHover && onHover(null)}
      >
        <span
          className="mr-1 w-4 h-4 flex items-center justify-center text-gray-500 hover:text-white"
          onClick={(e) => {
            e.stopPropagation();
            setExpanded(!expanded);
          }}
        >
          {item.children && item.children.length > 0 && (
            isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />
          )}
        </span>
        <span className="mr-2">{getIcon(item.type)}</span>
        <span className={`truncate flex-1 ${isSelected ? 'text-white font-medium' : 'text-gray-300'}`}>
          {renderLabel()}
        </span>
        {item.stats?.duration && (
          <span className="text-gray-500 ml-2 text-[10px] font-mono">{item.stats.duration}</span>
        )}
      </div>
      {isExpanded && item.children && (
        <div>
          {item.children.map((child) => (
            <TraceNodeItem
              key={child.id}
              item={child}
              depth={depth + 1}
              onSelect={onSelect}
              selectedId={selectedId}
              searchTerm={searchTerm}
              alwaysExpand={alwaysExpand}
              hoveredId={hoveredId}
              onHover={onHover}
            />
          ))}
        </div>
      )}
    </div>
  );
};

// Filter Logic
const filterTraceItems = (items: TraceItem[], term: string): { filtered: TraceItem[], count: number } => {
  if (!term) return { filtered: items, count: 0 };

  const lowerTerm = term.toLowerCase();
  const parts = lowerTerm.split(' ').filter(p => p.trim() !== '');

  // Parse modifiers
  const modifiers = parts.reduce((acc, part) => {
    if (part.startsWith('type:')) {
      acc.type = part.split(':')[1];
    } else if (part.startsWith('cost:>')) {
      acc.minCost = parseFloat(part.split(':>')[1]);
    } else if (part.startsWith('cost:<')) {
      acc.maxCost = parseFloat(part.split(':<')[1]);
    } else if (!part.includes(':')) {
      acc.text = part;
    }
    return acc;
  }, {} as { type?: string, minCost?: number, maxCost?: number, text?: string });

  let matchCount = 0;

  // Check if item matches
  const matches = (item: TraceItem) => {
    let isMatch = true;

    if (modifiers.type && !item.type.toLowerCase().includes(modifiers.type)) {
      isMatch = false;
    }

    if (modifiers.minCost !== undefined) {
      const duration = item.stats?.duration ? parseFloat(item.stats.duration.replace('ms', '')) : 0;
      if (duration <= modifiers.minCost) isMatch = false;
    }

    if (modifiers.maxCost !== undefined) {
      const duration = item.stats?.duration ? parseFloat(item.stats.duration.replace('ms', '')) : 0;
      if (duration >= modifiers.maxCost) isMatch = false;
    }

    if (modifiers.text) {
      const textMatch = item.label.toLowerCase().includes(modifiers.text) ||
        (item.description && item.description.toLowerCase().includes(modifiers.text));
      if (!textMatch) isMatch = false;
    }

    if (isMatch) matchCount++;
    return isMatch;
  };

  // Recursive filter
  const filter = (nodes: TraceItem[]): TraceItem[] => {
    return nodes.reduce((acc, node) => {
      const children = node.children ? filter(node.children) : [];
      const nodeMatches = matches(node);

      if (nodeMatches || children.length > 0) {
        acc.push({
          ...node,
          children: children
        });
      }
      return acc;
    }, [] as TraceItem[]);
  };

  const filtered = filter(items);
  return { filtered, count: matchCount };
};


interface SidebarProps {
  onSelect: (item: TraceItem) => void;
  selectedId: string | null;
  hoveredId?: string | null;
  onHover?: (id: string | null) => void;
  traceData: TraceItem[];
}

// ... (TraceNodeItem remains same) ...

export const Sidebar: React.FC<SidebarProps> = ({ onSelect, selectedId, hoveredId, onHover, traceData }) => {
  const [searchTerm, setSearchTerm] = useState('');

  const filteredData = useMemo(() => {
    return filterTraceItems(traceData, searchTerm);
  }, [searchTerm, traceData]);

  return (
    <div className="h-full flex flex-col bg-[#1e1e1e] border-r border-gray-800 text-gray-200 w-80 min-w-[300px] flex-shrink-0" data-testid="sidebar">
      {/* Header / Toolbar */}
      <div className="h-10 flex items-center px-3 border-b border-gray-800 bg-[#252526] gap-2">
        <SidebarIcon size={16} className="text-gray-400" />
        <span className="text-xs font-semibold text-gray-300">GPU Trace</span>
        <div className="flex-1" />
        <button className="p-1 hover:bg-gray-700 rounded"><Filter size={14} className="text-gray-400" /></button>
      </div>

      {/* Search */}
      <div className="p-2 border-b border-gray-800 bg-[#1e1e1e]">
        <div className="relative">
          <Search className="absolute left-2 top-1.5 text-gray-500" size={14} />
          <input
            type="text"
            placeholder="Filter (e.g. Squeeze, type:dispatch)"
            className="w-full bg-[#2d2d2d] text-gray-200 text-xs rounded border border-gray-700 pl-8 pr-2 py-1 focus:outline-none focus:border-blue-500"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          {searchTerm && (
            <div className="absolute right-2 top-1.5 flex items-center gap-2">
              <span className="text-[10px] text-gray-500">{filteredData.count} matches</span>
              <button
                className="text-gray-500 hover:text-white"
                onClick={() => setSearchTerm('')}
              >
                <span className="text-[10px]">✕</span>
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden py-2 custom-scrollbar">
        {filteredData.filtered.length === 0 ? (
          <div className="p-4 text-center text-gray-500 text-xs">
            No matching items found.
          </div>
        ) : (
          filteredData.filtered.map((item) => (
            <TraceNodeItem
              key={item.id}
              item={item}
              depth={0}
              onSelect={onSelect}
              selectedId={selectedId}
              searchTerm={searchTerm}
              alwaysExpand={!!searchTerm} // Auto-expand when searching
              hoveredId={hoveredId}
              onHover={onHover}
            />
          ))
        )}
      </div>

      {/* Summary Footer */}
      <div className="h-7 border-t border-gray-800 bg-[#252526] flex items-center px-3 text-[10px] text-gray-400 gap-4">
        <span>Memory: 942.97 MiB</span>
        <span>Time: 1.86 ms</span>
      </div>
    </div>
  );
};
