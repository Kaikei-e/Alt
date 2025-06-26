import { memo } from "react";

const SkeletonFeedCard = memo(function SkeletonFeedCard() {
  return (
    <div
      style={{
        padding: '2px',
        borderRadius: '18px',
        background: 'linear-gradient(45deg, rgba(255, 0, 110, 0.2), rgba(131, 56, 236, 0.2), rgba(58, 134, 255, 0.2))',
        marginBottom: '16px'
      }}
      data-testid="skeleton-feed-card"
    >
      <div
        style={{
          width: '100%',
          padding: '20px',
          borderRadius: '16px',
          backgroundColor: '#1a1a2e',
          opacity: 0.6
        }}
      >
        {/* Title skeleton */}
        <div
          style={{
            height: '24px',
            backgroundColor: 'rgba(255, 0, 110, 0.1)',
            borderRadius: '4px',
            width: '80%',
            marginBottom: '12px'
          }}
        />

        {/* Description skeleton */}
        <div
          style={{
            height: '16px',
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
            borderRadius: '4px',
            width: '100%',
            marginBottom: '8px'
          }}
        />
        <div
          style={{
            height: '16px',
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
            borderRadius: '4px',
            width: '70%',
            marginBottom: '16px'
          }}
        />

        {/* Bottom section skeleton */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div
            style={{
              height: '32px',
              width: '120px',
              backgroundColor: 'rgba(255, 0, 110, 0.1)',
              borderRadius: '16px'
            }}
          />
          <div
            style={{
              height: '32px',
              width: '100px',
              backgroundColor: 'rgba(255, 255, 255, 0.05)',
              borderRadius: '16px'
            }}
          />
        </div>
      </div>
    </div>
  );
});

export default SkeletonFeedCard;