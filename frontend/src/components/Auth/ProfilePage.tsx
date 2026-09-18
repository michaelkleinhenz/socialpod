import { useState } from 'react';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import { Key, Copy, RefreshCw, UsersRound, Lock, Server, ChevronDown, ChevronRight, Check } from 'lucide-react';
import toast from 'react-hot-toast';

export function ProfilePage() {
  const { user, refreshUser } = useAuth();
  const [apiToken, setApiToken] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [mcpExpanded, setMcpExpanded] = useState(false);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);

  const generateToken = async () => {
    setGenerating(true);
    try {
      const res = await api.generateApiToken();
      setApiToken(res.apiToken);
      refreshUser();
      toast.success('API token generated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setGenerating(false);
    }
  };

  const copyToClipboard = (text: string, label = 'Copied to clipboard') => {
    navigator.clipboard.writeText(text);
    toast.success(label);
  };

  const displayToken = apiToken || user?.apiToken || null;
  const mcpUrl = `${window.location.origin}/api/mcp`;

  const changePassword = async () => {
    if (!currentPassword || !newPassword || !confirmPassword) {
      toast.error('All fields are required');
      return;
    }
    if (newPassword.length < 8) {
      toast.error('New password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error('New passwords do not match');
      return;
    }
    setChangingPassword(true);
    try {
      await api.updatePassword(currentPassword, newPassword);
      toast.success('Password updated successfully');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setChangingPassword(false);
    }
  };

  const claudeCodeConfig = displayToken ? JSON.stringify({
    "mcpServers": {
      "socialpod": {
        "url": mcpUrl,
        "headers": {
          "Authorization": `Bearer ${displayToken}`
        }
      }
    }
  }, null, 2) : null;

  const openCodeConfig = displayToken ? JSON.stringify({
    "mcpServers": {
      "socialpod": {
        "url": mcpUrl,
        "headers": {
          "Authorization": `Bearer ${displayToken}`
        }
      }
    }
  }, null, 2) : null;

  const curlExample = displayToken
    ? `curl -X POST ${mcpUrl} \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${displayToken}" \\
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"1.0"}}}'`
    : null;

  return (
    <div className="page">
      <div className="page-header">
        <h1>Profile</h1>
      </div>

      <div className="card" style={{ maxWidth: 600 }}>
        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Name</label>
          <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user?.name}</div>
        </div>

        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Email</label>
          <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user?.email}</div>
        </div>

        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Role</label>
          <div>
            <span className={`badge ${user?.isAdmin ? 'badge-published' : 'badge-scheduled'}`}>
              {user?.isAdmin ? 'Admin' : 'User'}
            </span>
          </div>
        </div>

        {user?.teamName && (
          <div className="form-group" style={{ marginBottom: 20 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <UsersRound size={14} /> Team
            </label>
            <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user.teamName}</div>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Shared calendar and API token with your team
            </span>
          </div>
        )}

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

        <h3 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Lock size={18} /> Change Password
        </h3>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 16 }}>
          <div className="form-group">
            <label>Current Password</label>
            <input
              className="input"
              type="password"
              placeholder="Enter current password"
              value={currentPassword}
              onChange={e => setCurrentPassword(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label>New Password</label>
            <input
              className="input"
              type="password"
              placeholder="Min 8 characters"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label>Confirm New Password</label>
            <input
              className="input"
              type="password"
              placeholder="Repeat new password"
              value={confirmPassword}
              onChange={e => setConfirmPassword(e.target.value)}
            />
          </div>
        </div>

        <button className="btn btn-primary" onClick={changePassword} disabled={changingPassword}>
          {changingPassword ? 'Updating...' : 'Update Password'}
        </button>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

        <h3 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Key size={18} /> API Access
        </h3>
        <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
          Use an API token to access SocialPod from external tools, scripts, and MCP clients.
        </p>

        {displayToken && (
          <div style={{ marginBottom: 16 }}>
            <label style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 6, display: 'block' }}>
              Your API Token
            </label>
            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              fontFamily: 'monospace',
              fontSize: 13,
              wordBreak: 'break-all',
            }}>
              <span style={{ flex: 1 }}>{displayToken}</span>
              <button className="btn btn-ghost btn-sm" onClick={() => copyToClipboard(displayToken)}>
                <Copy size={14} />
              </button>
            </div>
          </div>
        )}

        <button className="btn btn-primary" onClick={generateToken} disabled={generating} style={{ marginBottom: 8 }}>
          <RefreshCw size={16} />
          {generating ? 'Generating...' : displayToken ? 'Regenerate Token' : 'Generate API Token'}
        </button>

        {displayToken && (
          <p style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 4, marginBottom: 0 }}>
            Regenerating will invalidate the current token.
          </p>
        )}

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

        <h3 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Server size={18} /> MCP Server
        </h3>
        <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
          Connect AI coding assistants to SocialPod using the Model Context Protocol (MCP).
          This gives AI tools access to manage your posts, footers, mentions, and accounts.
        </p>

        {displayToken && (
          <div style={{ marginBottom: 16 }}>
            <label style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 6, display: 'block' }}>
              MCP Server URL
            </label>
            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              fontFamily: 'monospace',
              fontSize: 13,
              wordBreak: 'break-all',
            }}>
              <span style={{ flex: 1 }}>{mcpUrl}</span>
              <button className="btn btn-ghost btn-sm" onClick={() => copyToClipboard(mcpUrl, 'URL copied')}>
                <Copy size={14} />
              </button>
            </div>
          </div>
        )}

        {!displayToken && (
          <div style={{
            background: 'var(--bg-primary)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            padding: '16px',
            marginBottom: 16,
            color: 'var(--text-secondary)',
            fontSize: 14,
          }}>
            Generate an API token above to see your MCP server configuration.
          </div>
        )}

        {displayToken && (
          <>
            <button
              onClick={() => setMcpExpanded(!mcpExpanded)}
              style={{
                background: 'none',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                padding: '12px 16px',
                width: '100%',
                textAlign: 'left',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                color: 'var(--text-primary)',
                fontSize: 14,
                fontWeight: 500,
              }}
            >
              {mcpExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
              Setup Instructions
            </button>

            {mcpExpanded && (
              <div style={{
                border: '1px solid var(--border)',
                borderTop: 'none',
                borderRadius: '0 0 var(--radius-sm) var(--radius-sm)',
                padding: '20px',
                background: 'var(--bg-primary)',
              }}>
                <MCPInstructions
                  title="Claude Code"
                  description={<>Add the following to your <code style={codeInlineStyle}>claude_desktop_config.json</code> or project <code style={codeInlineStyle}>.mcp.json</code> file:</>}
                  config={claudeCodeConfig!}
                  onCopy={copyToClipboard}
                />

                <MCPInstructions
                  title="OpenCode"
                  description={<>Add the following to your <code style={codeInlineStyle}>opencode.json</code> configuration:</>}
                  config={openCodeConfig!}
                  onCopy={copyToClipboard}
                />

                <MCPInstructions
                  title="Generic MCP Client"
                  description="Any MCP client supporting the Streamable HTTP transport can connect using:"
                  config={JSON.stringify({
                    url: mcpUrl,
                    transport: "streamable-http",
                    headers: {
                      Authorization: `Bearer ${displayToken}`
                    }
                  }, null, 2)}
                  onCopy={copyToClipboard}
                />

                <div style={{ marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--border)' }}>
                  <h4 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-primary)' }}>
                    Test with curl
                  </h4>
                  <CodeBlock code={curlExample!} onCopy={copyToClipboard} />
                </div>

                <div style={{ marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--border)' }}>
                  <h4 style={{ fontSize: 14, marginBottom: 8, color: 'var(--text-primary)' }}>
                    Available Tools
                  </h4>
                  <div style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.6 }}>
                    <ToolList />
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

const codeInlineStyle: React.CSSProperties = {
  background: 'var(--bg-secondary, rgba(0,0,0,0.1))',
  padding: '2px 6px',
  borderRadius: 4,
  fontSize: 12,
  fontFamily: 'monospace',
};

function CodeBlock({ code, onCopy }: { code: string; onCopy: (text: string, label?: string) => void }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    onCopy(code, 'Copied to clipboard');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div style={{ position: 'relative' }}>
      <pre style={{
        background: 'var(--bg-secondary, #1e1e2e)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-sm)',
        padding: '14px 16px',
        paddingRight: 48,
        fontSize: 12,
        fontFamily: 'monospace',
        overflowX: 'auto',
        whiteSpace: 'pre',
        lineHeight: 1.5,
        margin: 0,
        color: 'var(--text-primary)',
      }}>
        {code}
      </pre>
      <button
        className="btn btn-ghost btn-sm"
        onClick={handleCopy}
        style={{
          position: 'absolute',
          top: 8,
          right: 8,
          opacity: 0.7,
        }}
        title="Copy to clipboard"
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </button>
    </div>
  );
}

function MCPInstructions({
  title,
  description,
  config,
  onCopy,
}: {
  title: string;
  description: React.ReactNode;
  config: string;
  onCopy: (text: string, label?: string) => void;
}) {
  return (
    <div style={{ marginBottom: 20 }}>
      <h4 style={{ fontSize: 14, marginBottom: 6, color: 'var(--text-primary)' }}>
        {title}
      </h4>
      <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 8 }}>
        {description}
      </p>
      <CodeBlock code={config} onCopy={onCopy} />
    </div>
  );
}

function ToolList() {
  const tools = [
    { category: 'Posts', items: ['list_posts', 'get_post', 'create_post', 'update_post', 'delete_post', 'reschedule_post', 'retry_post'] },
    { category: 'Accounts', items: ['list_accounts'] },
    { category: 'Footers', items: ['list_footers', 'create_footer', 'update_footer', 'delete_footer'] },
    { category: 'Mentions', items: ['list_mentions', 'create_mention', 'update_mention', 'delete_mention'] },
    { category: 'Watermarks', items: ['list_watermarks', 'delete_watermark'] },
    { category: 'Profile', items: ['get_profile'] },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {tools.map(group => (
        <div key={group.category}>
          <strong style={{ color: 'var(--text-primary)', fontSize: 13 }}>{group.category}:</strong>{' '}
          {group.items.map((t, i) => (
            <span key={t}>
              <code style={{ ...codeInlineStyle, fontSize: 11 }}>{t}</code>
              {i < group.items.length - 1 ? ', ' : ''}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}
