import { TraceItem } from './types';

export const fetchTraceData = async (): Promise<TraceItem> => {
    const response = await fetch('/api/trace');
    if (!response.ok) {
        throw new Error(`Failed to fetch trace data: ${response.statusText}`);
    }
    return response.json();
};
