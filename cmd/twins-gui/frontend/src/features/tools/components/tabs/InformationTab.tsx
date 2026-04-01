import React, { useState, useEffect, useCallback, useRef } from 'react';
import { GetToolsInfo } from '@wailsjs/go/main/App';
import type { ToolsInfo } from '@/shared/types/tools.types';

const sectionHeaderStyle: React.CSSProperties = {
  padding: '10px 12px 4px 0',
  color: '#fff',
  fontWeight: 700,
  fontSize: '13px',
};

const labelStyle: React.CSSProperties = {
  padding: '6px 12px 6px 0',
  color: '#aaa',
  whiteSpace: 'nowrap',
  verticalAlign: 'top',
  width: '200px',
  fontSize: '13px',
};

const valueStyle: React.CSSProperties = {
  padding: '6px 0',
  color: '#fff',
  fontSize: '13px',
  wordBreak: 'break-all',
};

export const InformationTab: React.FC = () => {
  const [info, setInfo] = useState<ToolsInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const mountedRef = useRef(true);

  const fetchInfo = useCallback(async () => {
    try {
      const data = await GetToolsInfo();
      if (mountedRef.current) setInfo(data as ToolsInfo);
    } catch {
      // Silently handle - info will show as loading
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    fetchInfo();
    timerRef.current = setInterval(fetchInfo, 10000);
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [fetchInfo]);

  const formatUptime = (startupTime: number) => {
    const seconds = Math.floor(Date.now() / 1000 - startupTime);
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    return `${hours}h ${minutes}m ${secs}s`;
  };

  if (isLoading || !info) {
    return (
      <div style={{ padding: '20px', color: '#888' }}>Loading...</div>
    );
  }

  return (
    <div style={{ padding: '16px 20px', overflowY: 'auto', height: '100%' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <tbody>
          {/* General section */}
          <tr>
            <td colSpan={2} style={sectionHeaderStyle}>General</td>
          </tr>
          <tr>
            <td style={labelStyle}>Client name:</td>
            <td style={valueStyle}>{info.clientName}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Client version:</td>
            <td style={valueStyle}>{info.clientVersion}</td>
          </tr>
          <tr>
            <td style={{ ...labelStyle, paddingLeft: '10px' }}>Using database version:</td>
            <td style={valueStyle}>{info.databaseVersion}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Build date:</td>
            <td style={valueStyle}>{info.buildDate}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Go version:</td>
            <td style={valueStyle}>{info.goVersion}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Platform:</td>
            <td style={valueStyle}>{info.platform}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Startup time:</td>
            <td style={valueStyle}>{new Date(info.startupTime * 1000).toLocaleString()}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Uptime:</td>
            <td style={valueStyle}>{formatUptime(info.startupTime)}</td>
          </tr>

          {/* Network section */}
          <tr>
            <td colSpan={2} style={{ ...sectionHeaderStyle, paddingTop: '16px' }}>Network</td>
          </tr>
          <tr>
            <td style={labelStyle}>Name:</td>
            <td style={valueStyle}>{info.networkName}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Number of connections:</td>
            <td style={valueStyle}>{`${info.connections} (In: ${info.inPeers} / Out: ${info.outPeers})`}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Number of Masternodes:</td>
            <td style={valueStyle}>{String(info.masternodeCount)}</td>
          </tr>

          {/* Block chain section */}
          <tr>
            <td colSpan={2} style={{ ...sectionHeaderStyle, paddingTop: '16px' }}>Block chain</td>
          </tr>
          <tr>
            <td style={labelStyle}>Current number of blocks:</td>
            <td style={valueStyle}>{String(info.blockCount)}</td>
          </tr>
          <tr>
            <td style={labelStyle}>Last block time:</td>
            <td style={valueStyle}>{info.lastBlockTime ? new Date(info.lastBlockTime * 1000).toLocaleString() : 'N/A'}</td>
          </tr>

          {/* Debug log file section */}
          <tr>
            <td colSpan={2} style={{ ...sectionHeaderStyle, paddingTop: '16px' }}>Debug log file</td>
          </tr>
          <tr>
            <td style={labelStyle}>Data directory:</td>
            <td style={valueStyle}>{info.dataDir}</td>
          </tr>
          <tr>
            <td colSpan={2} style={{ padding: '8px 0' }}>
              <button
                disabled
                style={{
                  padding: '6px 16px',
                  backgroundColor: '#444',
                  color: '#666',
                  border: '1px solid #555',
                  borderRadius: '4px',
                  cursor: 'not-allowed',
                  fontSize: '13px',
                }}
              >
                Open
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
};
