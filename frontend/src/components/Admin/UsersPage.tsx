import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { User } from '../../types';
import { useAuth } from '../../contexts/AuthContext';
import { Trash2, Shield } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import './Admin.css';

export function UsersPage() {
  const { user: currentUser } = useAuth();
  const [users, setUsers] = useState<User[]>([]);

  useEffect(() => {
    api.getUsers().then(setUsers).catch(() => toast.error('Failed to load users'));
  }, []);

  const deleteUser = async (id: string) => {
    if (!confirm('Delete this user and all their posts?')) return;
    try {
      await api.deleteUser(id);
      setUsers(prev => prev.filter(u => u.id !== id));
      toast.success('User deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Users</h1>
      </div>

      <div className="card">
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Email</th>
                <th>Role</th>
                <th>Joined</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {users.map(user => (
                <tr key={user.id}>
                  <td style={{ fontWeight: 500 }}>{user.name}</td>
                  <td style={{ color: 'var(--text-secondary)' }}>{user.email}</td>
                  <td>
                    {user.isAdmin ? (
                      <span className="badge badge-published"><Shield size={10} /> Admin</span>
                    ) : (
                      <span className="badge badge-scheduled">User</span>
                    )}
                  </td>
                  <td style={{ color: 'var(--text-muted)', fontSize: 13 }}>
                    {format(new Date(user.createdAt), 'MMM d, yyyy')}
                  </td>
                  <td>
                    {user.id !== currentUser?.id && (
                      <button className="btn btn-ghost btn-sm" onClick={() => deleteUser(user.id)}>
                        <Trash2 size={14} color="var(--danger)" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
