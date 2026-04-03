"""Team 持久化模型。"""

from datetime import datetime

from sqlalchemy import Column, DateTime, ForeignKey, Integer, String, Text, UniqueConstraint
from sqlalchemy.orm import relationship

from .models import Account, Base, JSONEncodedDict


class Team(Base):
    """团队主表。"""

    __tablename__ = "teams"
    __table_args__ = (
        UniqueConstraint(
            "owner_account_id",
            "upstream_account_id",
            name="uq_team_owner_upstream_account",
        ),
    )

    id = Column(Integer, primary_key=True, autoincrement=True)
    owner_account_id = Column(Integer, ForeignKey("accounts.id"), nullable=False, index=True)
    upstream_team_id = Column(String(255), index=True)
    upstream_account_id = Column(String(255), nullable=False, index=True)
    team_name = Column(String(255), nullable=False)
    plan_type = Column(String(50), nullable=False)
    subscription_plan = Column(String(100))
    account_role_snapshot = Column(String(50))
    status = Column(String(50), default="pending")
    current_members = Column(Integer, default=0)
    max_members = Column(Integer)
    seats_available = Column(Integer)
    expires_at = Column(DateTime)
    last_sync_at = Column(DateTime)
    sync_status = Column(String(50), default="pending")
    sync_error = Column(Text)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    memberships = relationship("TeamMembership", back_populates="team", cascade="all, delete-orphan")
    tasks = relationship("TeamTask", back_populates="team", cascade="all, delete-orphan")


class TeamMembership(Base):
    """团队成员关系表。"""

    __tablename__ = "team_memberships"
    __table_args__ = (UniqueConstraint("team_id", "member_email", name="uq_team_member_email"),)

    id = Column(Integer, primary_key=True, autoincrement=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=False, index=True)
    local_account_id = Column(Integer, ForeignKey("accounts.id"), index=True)
    member_email = Column(String(255), nullable=False)
    upstream_user_id = Column(String(255), index=True)
    member_role = Column(String(50), default="member")
    membership_status = Column(String(50), default="pending")
    invited_at = Column(DateTime)
    joined_at = Column(DateTime)
    removed_at = Column(DateTime)
    last_seen_at = Column(DateTime)
    source = Column(String(50), default="sync")
    sync_error = Column(Text)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    team = relationship("Team", back_populates="memberships")


class TeamTask(Base):
    """团队任务表。"""

    __tablename__ = "team_tasks"

    id = Column(Integer, primary_key=True, autoincrement=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=True, index=True)
    owner_account_id = Column(Integer, ForeignKey("accounts.id"), nullable=True, index=True)
    task_uuid = Column(String(100), nullable=False, unique=True, index=True)
    task_type = Column(String(50), nullable=False)
    status = Column(String(50), default="pending")
    request_payload = Column(JSONEncodedDict)
    result_payload = Column(JSONEncodedDict)
    error_message = Column(Text)
    logs = Column(Text)
    created_at = Column(DateTime, default=datetime.utcnow)
    started_at = Column(DateTime)
    completed_at = Column(DateTime)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    team = relationship("Team", back_populates="tasks")
    items = relationship("TeamTaskItem", back_populates="task", cascade="all, delete-orphan")


class TeamTaskItem(Base):
    """团队任务子项表。"""

    __tablename__ = "team_task_items"
    __table_args__ = (UniqueConstraint("task_id", "target_email", name="uq_team_task_target_email"),)

    id = Column(Integer, primary_key=True, autoincrement=True)
    task_id = Column(Integer, ForeignKey("team_tasks.id"), nullable=False, index=True)
    target_email = Column(String(255), nullable=False)
    item_status = Column(String(50), default="pending")
    before_ = Column("before", JSONEncodedDict)
    after_ = Column("after", JSONEncodedDict)
    message = Column(Text)
    error_message = Column(Text)
    created_at = Column(DateTime, default=datetime.utcnow)
    started_at = Column(DateTime)
    completed_at = Column(DateTime)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    task = relationship("TeamTask", back_populates="items")
