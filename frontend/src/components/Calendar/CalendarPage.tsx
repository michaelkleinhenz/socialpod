import { useState, useEffect, useCallback } from 'react';
import {
  startOfMonth, endOfMonth, startOfWeek, endOfWeek, eachDayOfInterval,
  format, addMonths, subMonths, addWeeks, subWeeks, addDays, subDays,
  startOfDay, endOfDay, isSameMonth, isSameDay, isToday, parseISO,
} from 'date-fns';
import { DndContext, type DragEndEvent, DragOverlay, type DragStartEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { api } from '../../services/api';
import type { Post, PostType, SocialAccount } from '../../types';
import { PostEditor } from '../PostEditor/PostEditor';
import { CalendarPost } from './CalendarPost';
import { DraggablePost } from './DraggablePost';
import { DroppableDay } from './DroppableDay';
import { ChevronLeft, ChevronRight, Plus, Filter } from 'lucide-react';
import toast from 'react-hot-toast';
import './Calendar.css';

type ViewMode = 'month' | 'week' | '3day';

export function CalendarPage() {
  const [currentDate, setCurrentDate] = useState(new Date());
  const [viewMode, setViewMode] = useState<ViewMode>(() =>
    window.innerWidth <= 768 ? '3day' : 'month'
  );
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingPost, setEditingPost] = useState<Post | null>(null);
  const [selectedDate, setSelectedDate] = useState<Date | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [editorPostType, setEditorPostType] = useState<PostType>('post');
  const [filterPlatform, setFilterPlatform] = useState<string>('');
  const [filterStatus, setFilterStatus] = useState<string>('');
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  );

  // Month view date range
  const monthStart = startOfMonth(currentDate);
  const monthEnd = endOfMonth(currentDate);
  const calStart = startOfWeek(monthStart, { weekStartsOn: 1 });
  const calEnd = endOfWeek(monthEnd, { weekStartsOn: 1 });
  const days = eachDayOfInterval({ start: calStart, end: calEnd });

  // Week view date range
  const weekStart = startOfWeek(currentDate, { weekStartsOn: 1 });
  const weekEnd = endOfWeek(currentDate, { weekStartsOn: 1 });
  const weekDays = eachDayOfInterval({ start: weekStart, end: weekEnd });

  // 3-day view date range
  const threeDayStart = startOfDay(currentDate);
  const threeDayEnd = endOfDay(addDays(currentDate, 2));
  const threeDays = eachDayOfInterval({ start: threeDayStart, end: threeDayEnd });

  const fetchStart = viewMode === 'month' ? calStart : viewMode === '3day' ? threeDayStart : weekStart;
  const fetchEnd = viewMode === 'month' ? calEnd : viewMode === '3day' ? threeDayEnd : weekEnd;

  const fetchPosts = useCallback(async () => {
    try {
      const start = fetchStart.toISOString();
      const end = fetchEnd.toISOString();
      const data = await api.getPosts({ start, end, platform: filterPlatform, status: filterStatus });
      setPosts(data);
    } catch {
      toast.error('Failed to load posts');
    } finally {
      setLoading(false);
    }
  }, [currentDate, viewMode, filterPlatform, filterStatus]);

  useEffect(() => { fetchPosts(); }, [fetchPosts]);

  useEffect(() => { api.getActiveAccounts().then(setAccounts).catch(() => {}); }, []);

  const hasAccounts = accounts.length > 0;

  const getPostsForDay = (day: Date) =>
    posts.filter(p => isSameDay(parseISO(p.scheduledAt), day))
      .sort((a, b) => new Date(a.scheduledAt).getTime() - new Date(b.scheduledAt).getTime());

  const handlePrev = () => {
    if (viewMode === 'month') setCurrentDate(subMonths(currentDate, 1));
    else if (viewMode === '3day') setCurrentDate(subDays(currentDate, 3));
    else setCurrentDate(subWeeks(currentDate, 1));
  };

  const handleNext = () => {
    if (viewMode === 'month') setCurrentDate(addMonths(currentDate, 1));
    else if (viewMode === '3day') setCurrentDate(addDays(currentDate, 3));
    else setCurrentDate(addWeeks(currentDate, 1));
  };

  const navTitle = viewMode === 'month'
    ? format(currentDate, 'MMMM yyyy')
    : viewMode === '3day'
    ? `${format(threeDayStart, 'MMM d')} – ${format(threeDayEnd, 'MMM d, yyyy')}`
    : `${format(weekStart, 'MMM d')} – ${format(weekEnd, 'MMM d, yyyy')}`;

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(event.active.id as string);
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    setActiveId(null);
    const { active, over } = event;
    if (!over) return;

    const postId = active.id as string;
    const targetDate = over.id as string;
    const post = posts.find(p => p.id === postId);
    if (!post) return;

    const oldDate = parseISO(post.scheduledAt);
    const newDate = new Date(targetDate);
    // Keep the same time, change the date
    newDate.setHours(oldDate.getHours(), oldDate.getMinutes(), 0, 0);

    if (isSameDay(oldDate, newDate)) return;

    try {
      const updated = await api.reschedulePost(postId, newDate.toISOString());
      setPosts(prev => prev.map(p => p.id === postId ? updated : p));
      toast.success(`Moved to ${format(newDate, 'MMM d')}`);
    } catch {
      toast.error('Failed to reschedule');
    }
  };

  const handleCreatePost = (day?: Date, type: PostType = 'post') => {
    setEditingPost(null);
    setSelectedDate(day || null);
    setEditorPostType(type);
    setEditorOpen(true);
  };

  const handleEditPost = (post: Post) => {
    setEditingPost(post);
    setSelectedDate(null);
    setEditorPostType(post.postType || 'post');
    setEditorOpen(true);
  };

  const handleSavePost = async (data: any, files?: File[]) => {
    try {
      if (editingPost) {
        const updated = await api.updatePost(editingPost.id, data, files);
        setPosts(prev => prev.map(p => p.id === editingPost.id ? updated : p));
        toast.success('Post updated');
      } else {
        const created = await api.createPost(data, files);
        setPosts(prev => [...prev, created]);
        toast.success('Post created');
      }
      setEditorOpen(false);
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDeletePost = async (postId: string) => {
    try {
      await api.deletePost(postId);
      setPosts(prev => prev.filter(p => p.id !== postId));
      setEditorOpen(false);
      toast.success('Post deleted');
    } catch {
      toast.error('Failed to delete');
    }
  };

  const draggedPost = activeId ? posts.find(p => p.id === activeId) : null;

  return (
    <div className="page calendar-page">
      <div className="page-header">
        <div className="calendar-nav">
          <button className="btn btn-ghost" onClick={handlePrev}>
            <ChevronLeft size={20} />
          </button>
          <h1>{navTitle}</h1>
          <button className="btn btn-ghost" onClick={handleNext}>
            <ChevronRight size={20} />
          </button>
          <button className="btn btn-ghost btn-sm" onClick={() => setCurrentDate(new Date())}>
            Today
          </button>
        </div>

        <div className="calendar-actions">
          <div className="view-toggle">
            <button
              className={`view-toggle-btn ${viewMode === 'month' ? 'active' : ''}`}
              onClick={() => setViewMode('month')}
            >
              Month
            </button>
            <button
              className={`view-toggle-btn ${viewMode === '3day' ? 'active' : ''}`}
              onClick={() => setViewMode('3day')}
            >
              3 Day
            </button>
            <button
              className={`view-toggle-btn ${viewMode === 'week' ? 'active' : ''}`}
              onClick={() => setViewMode('week')}
            >
              Week
            </button>
          </div>
          <div className="filter-group">
            <Filter size={14} />
            <select className="select" style={{ width: 120 }} value={filterPlatform} onChange={e => setFilterPlatform(e.target.value)}>
              <option value="">All platforms</option>
              <option value="bluesky">Bluesky</option>
              <option value="instagram">Instagram</option>
              <option value="twitter">X/Twitter</option>
              <option value="mastodon">Mastodon</option>
              <option value="threads">Threads</option>
              <option value="linkedin">LinkedIn</option>
            </select>
            <select className="select" style={{ width: 120 }} value={filterStatus} onChange={e => setFilterStatus(e.target.value)}>
              <option value="">All status</option>
              <option value="scheduled">Scheduled</option>
              <option value="published">Published</option>
              <option value="draft">Draft</option>
              <option value="failed">Failed</option>
            </select>
          </div>
          <div className="new-post-actions">
            <button className="btn btn-primary" onClick={() => handleCreatePost()} disabled={!hasAccounts} title={!hasAccounts ? 'Add a social account to create posts' : undefined}>
              <Plus size={18} /> New Post
            </button>
            <button className="btn btn-secondary" onClick={() => handleCreatePost(undefined, 'story')} disabled={!hasAccounts} title={!hasAccounts ? 'Add a social account to create posts' : undefined}>
              <Plus size={18} /> New Story
            </button>
            <button className="btn btn-secondary" onClick={() => handleCreatePost(undefined, 'reel')} disabled={!hasAccounts} title={!hasAccounts ? 'Add a social account to create posts' : undefined}>
              <Plus size={18} /> New Reel
            </button>
          </div>
        </div>
      </div>

      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        {viewMode === '3day' ? (
          <div className="week-grid three-day-grid">
            <div className="calendar-header-row">
              {threeDays.map(day => (
                <div key={format(day, 'yyyy-MM-dd')} className="calendar-header-cell week-header-cell">
                  <span className="week-day-name">{format(day, 'EEE')}</span>
                  <span className={`week-day-number ${isToday(day) ? 'today-badge' : ''}`}>
                    {format(day, 'd')}
                  </span>
                </div>
              ))}
            </div>
            <div className="week-body">
              {threeDays.map(day => {
                const dayPosts = getPostsForDay(day);
                const dateStr = format(day, 'yyyy-MM-dd');
                return (
                  <DroppableDay key={dateStr} id={dateStr}>
                    <div
                      className={`week-day ${isToday(day) ? 'today' : ''}`}
                      onDoubleClick={() => handleCreatePost(day)}
                    >
                      <div className="day-posts">
                        {dayPosts.map(post => (
                          <DraggablePost key={post.id} id={post.id}>
                            <CalendarPost post={post} onClick={() => handleEditPost(post)} />
                          </DraggablePost>
                        ))}
                      </div>
                      <button className="add-post-btn week-add-btn" onClick={(e) => { e.stopPropagation(); handleCreatePost(day); }} disabled={!hasAccounts}>
                        <Plus size={14} />
                      </button>
                    </div>
                  </DroppableDay>
                );
              })}
            </div>
          </div>
        ) : viewMode === 'week' ? (
          <div className="week-grid">
            <div className="calendar-header-row">
              {weekDays.map(day => (
                <div key={format(day, 'yyyy-MM-dd')} className="calendar-header-cell week-header-cell">
                  <span className="week-day-name">{format(day, 'EEE')}</span>
                  <span className={`week-day-number ${isToday(day) ? 'today-badge' : ''}`}>
                    {format(day, 'd')}
                  </span>
                </div>
              ))}
            </div>
            <div className="week-body">
              {weekDays.map(day => {
                const dayPosts = getPostsForDay(day);
                const dateStr = format(day, 'yyyy-MM-dd');
                return (
                  <DroppableDay key={dateStr} id={dateStr}>
                    <div
                      className={`week-day ${isToday(day) ? 'today' : ''}`}
                      onDoubleClick={() => handleCreatePost(day)}
                    >
                      <div className="day-posts">
                        {dayPosts.map(post => (
                          <DraggablePost key={post.id} id={post.id}>
                            <CalendarPost post={post} onClick={() => handleEditPost(post)} />
                          </DraggablePost>
                        ))}
                      </div>
                      <button className="add-post-btn week-add-btn" onClick={(e) => { e.stopPropagation(); handleCreatePost(day); }} disabled={!hasAccounts}>
                        <Plus size={14} />
                      </button>
                    </div>
                  </DroppableDay>
                );
              })}
            </div>
          </div>
        ) : (
          <div className="calendar-grid">
            <div className="calendar-header-row">
              {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(d => (
                <div key={d} className="calendar-header-cell">{d}</div>
              ))}
            </div>

            <div className="calendar-body">
              {days.map(day => {
                const dayPosts = getPostsForDay(day);
                const dateStr = format(day, 'yyyy-MM-dd');

                return (
                  <DroppableDay key={dateStr} id={dateStr}>
                    <div
                      className={`calendar-day ${!isSameMonth(day, currentDate) ? 'other-month' : ''} ${isToday(day) ? 'today' : ''}`}
                      onDoubleClick={() => handleCreatePost(day)}
                    >
                      <div className="day-header">
                        <span className={`day-number ${isToday(day) ? 'today-badge' : ''}`}>
                          {format(day, 'd')}
                        </span>
                        {isSameMonth(day, currentDate) && (
                          <button className="add-post-btn" onClick={(e) => { e.stopPropagation(); handleCreatePost(day); }} disabled={!hasAccounts}>
                            <Plus size={14} />
                          </button>
                        )}
                      </div>
                      <div className="day-posts">
                        {dayPosts.map(post => (
                          <DraggablePost key={post.id} id={post.id}>
                            <CalendarPost post={post} onClick={() => handleEditPost(post)} />
                          </DraggablePost>
                        ))}
                      </div>
                    </div>
                  </DroppableDay>
                );
              })}
            </div>
          </div>
        )}

        <DragOverlay>
          {draggedPost && <CalendarPost post={draggedPost} onClick={() => {}} isDragging />}
        </DragOverlay>
      </DndContext>

      {loading && <div className="calendar-loading"><div className="spinner" /></div>}

      {editorOpen && (
        <PostEditor
          post={editingPost}
          postType={editorPostType}
          defaultDate={selectedDate}
          onSave={handleSavePost}
          onDelete={editingPost ? () => handleDeletePost(editingPost.id) : undefined}
          onClose={() => setEditorOpen(false)}
        />
      )}
    </div>
  );
}
