import { DesktopHomePage } from '../../components/desktop/home/DesktopHomePage';
import { ThemeProvider } from '../../providers/ThemeProvider';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';

export default function DesktopHomePageTest() {
  return (
    <ChakraProvider value={defaultSystem}>
      <ThemeProvider>
        <DesktopHomePage />
      </ThemeProvider>
    </ChakraProvider>
  );
}