"use client";

import { Box, Button, Heading, Input, Text, VStack } from "@chakra-ui/react";
import { useState } from "react";

export default function DesktopSettingsPage() {
  const [name, setName] = useState("Original Name");
  const [status, setStatus] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    // Simulate API call or use real API if available
    // For E2E test, the network request will be intercepted
    try {
      await fetch('/api/user/profile', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      setStatus("success");
    } catch (error) {
      console.error(error);
      setStatus("error");
    }
  };

  return (
    <Box p={8}>
      <Heading size="lg" mb={6}>
        Settings
      </Heading>
      <form onSubmit={handleSubmit}>
        <VStack gap={4} align="flex-start" maxW="md">
          <Box w="100%">
            <label htmlFor="name" style={{ display: 'block', marginBottom: '8px', fontWeight: 'bold' }}>Name</label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Box>
          <Button type="submit" colorPalette="blue">
            Save Changes
          </Button>
          {status === "success" && (
            <Text color="green.500">Profile updated.</Text>
          )}
        </VStack>
      </form>
    </Box>
  );
}
